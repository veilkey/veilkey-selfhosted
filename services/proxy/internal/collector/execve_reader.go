//go:build linux

package collector

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"time"
	"unsafe"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/events"
)

const (
	execveMaxArgs   = 32
	execveMaxArgLen = 1024
)

type execveEvent struct {
	PID       uint32
	UID       uint32
	Comm      [16]byte
	Argc      uint32
	Truncated uint8
	_         [3]byte
	Argv      [execveMaxArgs][execveMaxArgLen]byte
}

func (c *linuxCollector) initExecveReader() error {
	if c.execveReader != nil {
		return nil
	}

	var objs execveProbeObjects
	if err := loadExecveProbeObjects(&objs, nil); err != nil {
		return fmt.Errorf("load execve probe objects: %w", err)
	}

	tp, err := link.Tracepoint("syscalls", "sys_enter_execve", objs.TraceEnterExecve, nil)
	if err != nil {
		_ = objs.Close()
		return fmt.Errorf("attach sys_enter_execve tracepoint: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.ExecveEvents)
	if err != nil {
		_ = tp.Close()
		_ = objs.Close()
		return fmt.Errorf("open execve ringbuf: %w", err)
	}

	c.execveObjs = &objs
	c.execveTP = tp
	c.execveReader = reader
	return nil
}

func (c *linuxCollector) observeExec(ctx context.Context, emit func(events.Event)) error {
	if err := c.initExecveReader(); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		if c.execveReader != nil {
			_ = c.execveReader.Close()
		}
	}()

	for {
		record, err := c.execveReader.Read()
		if err != nil {
			if ctx.Err() != nil || strings.Contains(err.Error(), "closed") {
				return nil
			}
			return fmt.Errorf("read execve ringbuf: %w", err)
		}

		expectedSize := int(unsafe.Sizeof(execveEvent{}))
		if len(record.RawSample) < expectedSize {
			log.Printf("execve: undersized record (%d < %d), skipping", len(record.RawSample), expectedSize)
			continue
		}

		var raw execveEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &raw); err != nil {
			log.Printf("execve: decode error: %v, skipping", err)
			continue
		}

		if c.cfg.TargetUID != 0 && uint(raw.UID) != c.cfg.TargetUID {
			continue
		}

		ev := events.Event{
			Time:       time.Now(),
			Kind:       events.KindExecve,
			PID:        raw.PID,
			UID:        raw.UID,
			Comm:       strings.TrimRight(string(raw.Comm[:]), "\x00"),
			CgroupPath: procCgroupPath(raw.PID),
			Argv:       decodeExecveArgv(raw),
			Truncated:  raw.Truncated != 0,
		}
		if len(ev.Argv) > 0 {
			ev.Comm = ev.Argv[0]
		}
		if c.cfg.TargetCgroup != "" && !strings.Contains(ev.CgroupPath, normalizeCgroupMatch(c.cfg.TargetCgroup)) {
			continue
		}
		emit(ev)
	}
}

func decodeExecveArgv(raw execveEvent) []string {
	argc := int(raw.Argc)
	if argc > execveMaxArgs {
		argc = execveMaxArgs
	}

	argv := make([]string, 0, argc)
	for i := 0; i < argc; i++ {
		arg := cString(raw.Argv[i][:])
		if arg == "" {
			continue
		}
		argv = append(argv, arg)
	}
	return argv
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

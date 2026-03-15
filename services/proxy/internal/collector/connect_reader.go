//go:build linux

package collector

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
	"unsafe"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/events"
)

type connectEvent struct {
	PID    uint32
	UID    uint32
	Family uint32
	Port   uint16
	Comm   [16]byte
	_      uint16
	Addr4  uint32
	Addr6  [16]byte
}

func (c *linuxCollector) initConnectReader() error {
	if c.connectReader != nil {
		return nil
	}

	var objs connectProbeObjects
	if err := loadConnectProbeObjects(&objs, nil); err != nil {
		return fmt.Errorf("load connect probe objects: %w", err)
	}

	tp, err := link.Tracepoint("syscalls", "sys_enter_connect", objs.TraceEnterConnect, nil)
	if err != nil {
		_ = objs.Close()
		return fmt.Errorf("attach sys_enter_connect tracepoint: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.ConnectEvents)
	if err != nil {
		_ = tp.Close()
		_ = objs.Close()
		return fmt.Errorf("open connect ringbuf: %w", err)
	}

	c.connectObjs = &objs
	c.connectTP = tp
	c.connectReader = reader
	return nil
}

func (c *linuxCollector) observeConnect(ctx context.Context, emit func(events.Event)) error {
	if err := c.initConnectReader(); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		if c.connectReader != nil {
			_ = c.connectReader.Close()
		}
	}()

	for {
		record, err := c.connectReader.Read()
		if err != nil {
			if ctx.Err() != nil || strings.Contains(err.Error(), "closed") {
				return nil
			}
			return fmt.Errorf("read connect ringbuf: %w", err)
		}

		expectedSize := int(unsafe.Sizeof(connectEvent{}))
		if len(record.RawSample) < expectedSize {
			log.Printf("connect: undersized record (%d < %d), skipping", len(record.RawSample), expectedSize)
			continue
		}

		var raw connectEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &raw); err != nil {
			log.Printf("connect: decode error: %v, skipping", err)
			continue
		}

		if c.cfg.TargetUID != 0 && uint(raw.UID) != c.cfg.TargetUID {
			continue
		}

		targetAddr := ""
		switch raw.Family {
		case 2:
			ip := make(net.IP, 4)
			binary.LittleEndian.PutUint32(ip, raw.Addr4)
			targetAddr = net.JoinHostPort(ip.String(), fmt.Sprintf("%d", raw.Port))
		case 10:
			ip := net.IP(raw.Addr6[:])
			targetAddr = net.JoinHostPort(ip.String(), fmt.Sprintf("%d", raw.Port))
		default:
			continue
		}

		ev := events.Event{
			Time:       time.Now(),
			Kind:       events.KindConnect,
			PID:        raw.PID,
			UID:        raw.UID,
			Comm:       strings.TrimRight(string(raw.Comm[:]), "\x00"),
			CgroupPath: procCgroupPath(raw.PID),
			TargetAddr: targetAddr,
		}
		if c.cfg.TargetCgroup != "" && !strings.Contains(ev.CgroupPath, normalizeCgroupMatch(c.cfg.TargetCgroup)) {
			continue
		}
		emit(ev)
	}
}

//go:build linux

package collector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/config"
	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/events"
)

func TestDecodeExecveArgv(t *testing.T) {
	var raw execveEvent
	raw.Argc = 4
	copy(raw.Argv[0][:], []byte("/bin/echo"))
	copy(raw.Argv[1][:], []byte("alpha"))
	copy(raw.Argv[2][:], []byte("beta"))

	got := decodeExecveArgv(raw)
	if len(got) != 3 {
		t.Fatalf("decodeExecveArgv() len = %d, want 3", len(got))
	}
	if got[0] != "/bin/echo" || got[1] != "alpha" || got[2] != "beta" {
		t.Fatalf("decodeExecveArgv() = %#v", got)
	}
}

func TestDecodeExecveArgvStopsAtFirstNUL(t *testing.T) {
	var raw execveEvent
	raw.Argc = 1
	copy(raw.Argv[0][:], []byte{'c', 'u', 'r', 'l', 0, 'x', 'x', 'x'})

	got := decodeExecveArgv(raw)
	if len(got) != 1 {
		t.Fatalf("decodeExecveArgv() len = %d, want 1", len(got))
	}
	if got[0] != "curl" {
		t.Fatalf("decodeExecveArgv()[0] = %q, want %q", got[0], "curl")
	}
}

func TestObserveCapturesExecveEvent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := os.Stat("/sys/fs/bpf"); err != nil {
		t.Skipf("requires /sys/fs/bpf: %v", err)
	}

	marker := "vk-observe-test-" + time.Now().Format("20060102150405.000000000")
	cfg := config.Default()
	cfg.TargetUID = uint(os.Geteuid())
	ev, ok := captureExecEvent(t, cfg, marker)
	if !ok {
		t.Fatalf("timed out waiting for execve event for marker %q", marker)
	}

	if ev.PID == 0 {
		t.Fatalf("captured event PID = 0")
	}
	if ev.UID != uint32(os.Geteuid()) {
		t.Fatalf("captured event UID = %d, want %d", ev.UID, os.Geteuid())
	}
	if len(ev.Argv) == 0 {
		t.Fatalf("captured event argv is empty")
	}
	if ev.Argv[0] != "/bin/echo" {
		t.Fatalf("captured argv[0] = %q, want %q", ev.Argv[0], "/bin/echo")
	}
	if ev.Comm != "/bin/echo" {
		t.Fatalf("captured comm = %q, want %q", ev.Comm, "/bin/echo")
	}
}

func TestObserveExecRespectsTargetUIDFilter(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := os.Stat("/sys/fs/bpf"); err != nil {
		t.Skipf("requires /sys/fs/bpf: %v", err)
	}

	cfg := config.Default()
	cfg.TargetUID = 424242

	marker := "vk-observe-uid-filter-" + time.Now().Format("20060102150405.000000000")
	if ev, ok := captureExecEvent(t, cfg, marker); ok {
		t.Fatalf("unexpected event matched under wrong uid filter: %#v", ev)
	}
}

func TestObserveExecRespectsTargetCgroupFilter(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := os.Stat("/sys/fs/bpf"); err != nil {
		t.Skipf("requires /sys/fs/bpf: %v", err)
	}

	cfg := config.Default()
	cfg.TargetUID = uint(os.Geteuid())
	cfg.TargetCgroup = "/definitely/not/the/current/cgroup"

	marker := "vk-observe-cgroup-filter-" + time.Now().Format("20060102150405.000000000")
	if ev, ok := captureExecEvent(t, cfg, marker); ok {
		t.Fatalf("unexpected event matched under wrong cgroup filter: %#v", ev)
	}
}

func TestObserveExecCapturesCurrentCgroupPath(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := os.Stat("/sys/fs/bpf"); err != nil {
		t.Skipf("requires /sys/fs/bpf: %v", err)
	}

	currentCgroup := procCgroupPath(uint32(os.Getpid()))
	if currentCgroup == "" {
		t.Skip("current process cgroup path unavailable")
	}

	cfg := config.Default()
	cfg.TargetUID = uint(os.Geteuid())
	cfg.TargetCgroup = filepath.Join("/sys/fs/cgroup", currentCgroup)

	marker := "vk-observe-cgroup-ok-" + time.Now().Format("20060102150405.000000000")
	ev, ok := captureExecEvent(t, cfg, marker)
	if !ok {
		t.Fatalf("timed out waiting for execve event under matching cgroup filter")
	}
	if !strings.Contains(ev.CgroupPath, currentCgroup) {
		t.Fatalf("captured cgroup path %q does not contain %q", ev.CgroupPath, currentCgroup)
	}
}

func TestDecodeExecveArgvBoundsOverflow(t *testing.T) {
	var raw execveEvent
	// argc exceeds MAX_ARGS — must be clamped
	raw.Argc = 999
	copy(raw.Argv[0][:], []byte("/bin/sh"))

	got := decodeExecveArgv(raw)
	if len(got) > execveMaxArgs {
		t.Fatalf("decodeExecveArgv() returned %d args, limit is %d", len(got), execveMaxArgs)
	}
	if len(got) == 0 || got[0] != "/bin/sh" {
		t.Fatalf("decodeExecveArgv() first arg = %q, want /bin/sh", got[0])
	}
}

func TestDecodeExecveArgvZeroArgc(t *testing.T) {
	var raw execveEvent
	raw.Argc = 0
	// Even with data in argv buffers, zero argc should yield empty result
	copy(raw.Argv[0][:], []byte("should-not-appear"))

	got := decodeExecveArgv(raw)
	if len(got) != 0 {
		t.Fatalf("decodeExecveArgv() with argc=0 returned %d args, want 0", len(got))
	}
}

func TestCStringNoNullTerminator(t *testing.T) {
	// Buffer with no null byte — should return entire buffer as string
	buf := make([]byte, 8)
	for i := range buf {
		buf[i] = 'A'
	}
	got := cString(buf)
	if len(got) != 8 {
		t.Fatalf("cString() with no NUL returned len %d, want 8", len(got))
	}
}

func containsArg(argv []string, needle string) bool {
	for _, arg := range argv {
		if strings.Contains(arg, needle) {
			return true
		}
	}
	return false
}

func captureExecEvent(t *testing.T, cfg config.Config, marker string) (events.Event, bool) {
	t.Helper()

	rawCollector, err := newLinuxCollector(cfg)
	if err != nil {
		t.Fatalf("newLinuxCollector() error = %v", err)
	}
	c := rawCollector.(*linuxCollector)
	defer c.Close()

	if err := c.Preflight(); err != nil {
		t.Fatalf("Preflight() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	matchCh := make(chan events.Event, 1)
	errCh := make(chan error, 1)

	go func() {
		errCh <- c.observeExec(ctx, func(ev events.Event) {
			if ev.Kind != events.KindExecve {
				return
			}
			if !containsArg(ev.Argv, marker) {
				return
			}
			select {
			case matchCh <- ev:
			default:
			}
			cancel()
		})
	}()

	time.Sleep(300 * time.Millisecond)

	cmd := exec.Command("/bin/echo", marker)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("echo marker failed: %v, output=%s", err, string(out))
	}

	wait := time.NewTimer(5 * time.Second)
	defer wait.Stop()

	for {
		select {
		case ev := <-matchCh:
			return ev, true
		case err := <-errCh:
			select {
			case ev := <-matchCh:
				return ev, true
			default:
			}
			if err != nil {
				t.Fatalf("observeExec() error = %v", err)
			}
			return events.Event{}, false
		case <-wait.C:
			cancel()
			return events.Event{}, false
		}
	}
}

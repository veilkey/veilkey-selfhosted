package detector

import (
	"strings"
	"testing"

	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/events"
)

func TestApplyExecArgv(t *testing.T) {
	d := New()
	ev := events.Event{
		Kind: events.KindExecve,
		Argv: []string{"curl", "-H", "Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz123456"},
	}

	got := d.Apply(ev)
	if !got.Suspicious {
		t.Fatal("expected event to be suspicious")
	}
	if len(got.Matches) == 0 {
		t.Fatal("expected detector matches")
	}
}

func TestApplyNoMatch(t *testing.T) {
	d := New()
	ev := events.Event{
		Kind: events.KindExecve,
		Argv: []string{"curl", "https://example.com"},
	}

	got := d.Apply(ev)
	if got.Suspicious {
		t.Fatal("expected event to be clean")
	}
}

func TestDetectorLargeArgv(t *testing.T) {
	d := New()
	bigArg := strings.Repeat("x", 2<<20)
	ev := events.Event{
		Comm: "test",
		Argv: []string{bigArg},
	}
	result := d.Apply(ev)
	if result.Suspicious {
		t.Fatal("large benign input should not be suspicious")
	}
}

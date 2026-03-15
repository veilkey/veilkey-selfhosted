package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/collector"
	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/config"
	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/detector"
	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/enforcer"
	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/events"
)

func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "observe":
		return runObserve(args[1:])
	case "doctor":
		return runDoctor()
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func runObserve(args []string) error {
	cfg := config.Default()
	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.UintVar(&cfg.TargetUID, "uid", cfg.TargetUID, "target uid filter (0 = disabled)")
	fs.StringVar(&cfg.TargetCgroup, "cgroup", cfg.TargetCgroup, "target cgroup path filter")
	fs.StringVar(&cfg.Format, "format", cfg.Format, "output format: text|json")
	fs.BoolVar(&cfg.Once, "once", cfg.Once, "run preflight only and exit")
	fs.BoolVar(&cfg.OnlySuspicious, "only-suspicious", cfg.OnlySuspicious, "emit only suspicious events")
	fs.BoolVar(&cfg.EnforceKill, "enforce-kill", cfg.EnforceKill, "kill suspicious execve processes after detection")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	c, err := collector.New(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := c.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "collector close: %v\n", closeErr)
		}
	}()

	if err := c.Preflight(); err != nil {
		return err
	}
	if cfg.Once {
		fmt.Println("ok: preflight passed")
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	d := detector.New()
	e := enforcer.New(cfg)
	fmt.Fprintln(os.Stderr, "observing exec and connect events")
	return c.Observe(ctx, func(ev events.Event) {
		ev = d.Apply(ev)
		ev = e.Apply(ev)
		if cfg.OnlySuspicious && !ev.Suspicious {
			return
		}
		printEvent(cfg.Format, ev)
	})
}

func runDoctor() error {
	cfg := config.Default()
	c, err := collector.New(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := c.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "collector close: %v\n", closeErr)
		}
	}()

	if err := c.Preflight(); err != nil {
		return err
	}
	fmt.Println("ok: linux collector preflight passed")
	return nil
}

func printUsage() {
	fmt.Print(`veilkey-proxy

Usage:
  veilkey-proxy observe [--uid <uid>] [--cgroup <path>] [--format text|json] [--only-suspicious] [--enforce-kill] [--once]
  veilkey-proxy doctor
  veilkey-proxy help
`)
}

func printEvent(format string, ev events.Event) {
	switch format {
	case "json":
		if err := json.NewEncoder(os.Stdout).Encode(ev); err != nil {
			fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
		}
	default:
		fmt.Printf("%s kind=%s uid=%d pid=%d comm=%q cgroup=%q target=%q suspicious=%t matches=%q enforcement=%q argv=%q\n",
			ev.Time.Format(time.RFC3339),
			ev.Kind,
			ev.UID,
			ev.PID,
			ev.Comm,
			ev.CgroupPath,
			ev.TargetAddr,
			ev.Suspicious,
			ev.Matches,
			enforcer.Describe(ev),
			ev.Argv,
		)
	}
}

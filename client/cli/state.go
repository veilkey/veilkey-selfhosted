package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	pasteModeOn  = "on"
	pasteModeOff = "off"
)

func stateDir() string {
	return getEnv("VEILKEY_STATE_DIR", defaultStateDir())
}

func pasteModePath() string {
	return stateDir() + "/paste-mode"
}

func currentPasteMode() string {
	data, err := os.ReadFile(pasteModePath())
	if err != nil {
		return pasteModeOn
	}
	mode := strings.ToLower(strings.TrimSpace(string(data)))
	if mode == pasteModeOff {
		return pasteModeOff
	}
	return pasteModeOn
}

func pasteTempIssuanceEnabled() bool {
	return currentPasteMode() == pasteModeOn
}

func setPasteMode(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != pasteModeOn && mode != pasteModeOff {
		return fmt.Errorf("invalid paste mode: %s", mode)
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pasteModePath(), []byte(mode+"\n"), 0o644)
}

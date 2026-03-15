package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type systemUpdateState struct {
	Status         string     `json:"status"`
	LastError      string     `json:"last_error,omitempty"`
	LastStartedAt  *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt *time.Time `json:"last_finished_at,omitempty"`
	LastTarget     string     `json:"last_target,omitempty"`
}

type systemUpdatePayload struct {
	CurrentVersion  string            `json:"current_version"`
	TargetVersion   string            `json:"target_version"`
	ReleaseChannel  string            `json:"release_channel"`
	UpdateEnabled   bool              `json:"update_enabled"`
	UpdateRunning   bool              `json:"update_running"`
	UpdateAvailable bool              `json:"update_available"`
	ScriptPath      string            `json:"script_path,omitempty"`
	State           systemUpdateState `json:"state"`
}

func currentProductVersion() string {
	if value := strings.TrimSpace(os.Getenv("VEILKEY_PRODUCT_VERSION")); value != "" {
		return value
	}
	if file := strings.TrimSpace(os.Getenv("VEILKEY_PRODUCT_VERSION_FILE")); file != "" {
		if data, err := os.ReadFile(file); err == nil {
			if value := strings.TrimSpace(string(data)); value != "" {
				return value
			}
		}
	}
	candidates := []string{
		"VERSION",
		filepath.Join("..", "VERSION"),
		filepath.Join("..", "..", "VERSION"),
		filepath.Join("..", "..", "..", "VERSION"),
	}
	for _, candidate := range candidates {
		if data, err := os.ReadFile(candidate); err == nil {
			if value := strings.TrimSpace(string(data)); value != "" {
				return value
			}
		}
	}
	return "unknown"
}

func updateScriptPath() string {
	return strings.TrimSpace(os.Getenv("VEILKEY_UPDATE_SCRIPT"))
}

func updateWorkdir() string {
	return strings.TrimSpace(os.Getenv("VEILKEY_UPDATE_WORKDIR"))
}

func updateTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("VEILKEY_UPDATE_TIMEOUT")); raw != "" {
		if dur, err := time.ParseDuration(raw); err == nil {
			return dur
		}
	}
	return 30 * time.Minute
}

func (s *Server) snapshotSystemUpdate() systemUpdateState {
	s.updateMu.RLock()
	defer s.updateMu.RUnlock()
	return s.updateState
}

func (s *Server) handleGetSystemUpdate(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load ui config")
		return
	}
	current := currentProductVersion()
	target := strings.TrimSpace(cfg.TargetVersion)
	payload := systemUpdatePayload{
		CurrentVersion:  current,
		TargetVersion:   target,
		ReleaseChannel:  cfg.ReleaseChannel,
		UpdateEnabled:   updateScriptPath() != "",
		UpdateRunning:   s.snapshotSystemUpdate().Status == "running",
		UpdateAvailable: target != "" && target != current,
		ScriptPath:      updateScriptPath(),
		State:           s.snapshotSystemUpdate(),
	}
	s.respondJSON(w, http.StatusOK, payload)
}

func (s *Server) handleRunSystemUpdate(w http.ResponseWriter, r *http.Request) {
	scriptPath := updateScriptPath()
	if scriptPath == "" {
		s.respondError(w, http.StatusServiceUnavailable, "update script is not configured")
		return
	}
	if _, err := os.Stat(scriptPath); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, "update script is not available")
		return
	}
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load ui config")
		return
	}
	target := strings.TrimSpace(cfg.TargetVersion)
	if target == "" {
		s.respondError(w, http.StatusBadRequest, "target_version is required before update")
		return
	}

	s.updateMu.Lock()
	if s.updateState.Status == "running" {
		s.updateMu.Unlock()
		s.respondError(w, http.StatusConflict, "update is already running")
		return
	}
	startedAt := time.Now().UTC()
	s.updateState = systemUpdateState{
		Status:        "running",
		LastStartedAt: &startedAt,
		LastTarget:    target,
	}
	s.updateMu.Unlock()

	go s.runSystemUpdate(scriptPath, cfg.ReleaseChannel, target)
	s.respondJSON(w, http.StatusAccepted, map[string]any{
		"status":          "started",
		"target_version":  target,
		"release_channel": cfg.ReleaseChannel,
		"started_at":      startedAt,
	})
}

func (s *Server) runSystemUpdate(scriptPath, channel, target string) {
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, scriptPath)
	if workdir := updateWorkdir(); workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Env = append(os.Environ(),
		"VEILKEY_UPDATE_TARGET_VERSION="+target,
		"VEILKEY_UPDATE_RELEASE_CHANNEL="+channel,
		"VEILKEY_UPDATE_CURRENT_VERSION="+currentProductVersion(),
	)
	err := cmd.Run()

	finishedAt := time.Now().UTC()
	next := systemUpdateState{
		Status:         "succeeded",
		LastStartedAt:  s.snapshotSystemUpdate().LastStartedAt,
		LastFinishedAt: &finishedAt,
		LastTarget:     target,
	}
	if err != nil {
		next.Status = "failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			next.LastError = "update command timed out"
		} else {
			next.LastError = err.Error()
		}
	}

	s.updateMu.Lock()
	s.updateState = next
	s.updateMu.Unlock()
}

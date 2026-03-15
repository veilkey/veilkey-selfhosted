package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"veilkey-keycenter/internal/db"
)

var functionRunAllowlist = map[string]struct{}{
	"curl":                    {},
	"gh":                      {},
	"git":                     {},
	"glab":                    {},
	"veilkey-gemini-frontend": {},
}

var functionRunPlaceholderRe = regexp.MustCompile(`\{\%\{([A-Za-z_][A-Za-z0-9_]*)\}\%\}`)

var functionRunEnvAllowlist = map[string]struct{}{
	"VEILKEY_GEMINI_FRONTEND_SYSTEM":   {},
	"VEILKEY_GEMINI_TEMPERATURE":       {},
	"VEILKEY_GEMINI_MAX_OUTPUT_TOKENS": {},
}

const (
	defaultFunctionRunTimeout = 120 * time.Second
	maxFunctionRunTimeout     = 10 * time.Minute
)

type globalFunctionVarSpec struct {
	Ref string `json:"ref"`
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func (s *Server) localFunctionBaseURL() string {
	addr := strings.TrimSpace(os.Getenv("VEILKEY_ADDR"))
	tlsCert := os.Getenv("VEILKEY_TLS_CERT")
	scheme := "http"
	if tlsCert != "" {
		scheme = "https"
	}
	if addr == "" {
		return scheme + "://127.0.0.1:10180"
	}
	if strings.HasPrefix(addr, ":") {
		return scheme + "://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return scheme + "://" + addr
}

func (s *Server) resolveFunctionRef(ref string) (string, error) {
	baseURL := s.localFunctionBaseURL()
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/resolve/"+ref, nil)
	if err != nil {
		return "", fmt.Errorf("resolve %s request build failed: %w", ref, err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve %s failed: %w", ref, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", fmt.Errorf("resolve %s response read failed: %w", ref, readErr)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("resolve %s failed: %s", ref, msg)
	}
	var payload struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("resolve %s failed: %w", ref, err)
	}
	return strings.TrimSpace(payload.Value), nil
}

func (s *Server) renderGlobalFunctionCommand(fn *db.GlobalFunction) (string, error) {
	if fn == nil {
		return "", fmt.Errorf("global function is required")
	}
	fields := strings.Fields(fn.Command)
	if len(fields) == 0 {
		return "", fmt.Errorf("function command is empty")
	}
	if _, ok := functionRunAllowlist[fields[0]]; !ok {
		return "", fmt.Errorf("function command %q is not allowed", fields[0])
	}

	vars := map[string]globalFunctionVarSpec{}
	if strings.TrimSpace(fn.VarsJSON) != "" {
		if err := json.Unmarshal([]byte(fn.VarsJSON), &vars); err != nil {
			return "", fmt.Errorf("invalid vars_json: %w", err)
		}
	}

	var renderErr error
	rendered := functionRunPlaceholderRe.ReplaceAllStringFunc(fn.Command, func(token string) string {
		match := functionRunPlaceholderRe.FindStringSubmatch(token)
		if len(match) != 2 {
			renderErr = fmt.Errorf("invalid placeholder token: %s", token)
			return ""
		}
		spec, ok := vars[match[1]]
		if !ok || strings.TrimSpace(spec.Ref) == "" {
			renderErr = fmt.Errorf("missing ref for placeholder %s", match[1])
			return ""
		}
		value, err := s.resolveFunctionRef(spec.Ref)
		if err != nil {
			renderErr = err
			return ""
		}
		return shellQuote(value)
	})
	if renderErr != nil {
		return "", renderErr
	}
	return rendered, nil
}

func (s *Server) buildGlobalFunctionRunEnv(req globalFunctionRunRequest) ([]string, []string, error) {
	env := append(os.Environ(),
		"VEILKEY_LOCALVAULT_URL="+s.localFunctionBaseURL(),
		"VEILKEY_FUNCTION_DIR=/opt/veilkey/veilkey-cli/functions",
	)
	applied := []string{}
	apply := func(key, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		env = append(env, key+"="+value)
		applied = append(applied, key)
	}
	apply("VEILKEY_GEMINI_FRONTEND_SYSTEM", req.SystemPrompt)
	if req.Temperature != nil {
		apply("VEILKEY_GEMINI_TEMPERATURE", fmt.Sprintf("%g", *req.Temperature))
	}
	if req.MaxOutputTokens != nil {
		apply("VEILKEY_GEMINI_MAX_OUTPUT_TOKENS", fmt.Sprintf("%d", *req.MaxOutputTokens))
	}
	for key, value := range req.Env {
		if _, ok := functionRunEnvAllowlist[key]; !ok {
			return nil, nil, fmt.Errorf("env override %q is not allowed", key)
		}
		apply(key, value)
	}
	return env, applied, nil
}

func (s *Server) resolveGlobalFunctionRunTimeout(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		return defaultFunctionRunTimeout
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout > maxFunctionRunTimeout {
		return maxFunctionRunTimeout
	}
	return timeout
}

type globalFunctionRunRequest struct {
	Prompt          string            `json:"prompt"`
	Stdin           string            `json:"stdin"`
	Input           string            `json:"input"`
	SystemPrompt    string            `json:"system_prompt"`
	Temperature     *float64          `json:"temperature"`
	MaxOutputTokens *int              `json:"max_output_tokens"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
	Env             map[string]string `json:"env"`
}

func (s *Server) handleGlobalFunctionRun(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "function name is required")
		return
	}
	fn, err := s.db.GetGlobalFunction(name)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	var req globalFunctionRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	stdin := req.Stdin
	if stdin == "" {
		stdin = req.Input
	}
	if stdin == "" {
		stdin = req.Prompt
	}

	rendered, err := s.renderGlobalFunctionCommand(fn)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	env, appliedEnvKeys, err := s.buildGlobalFunctionRunEnv(req)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	timeout := s.resolveGlobalFunctionRunTimeout(req.TimeoutSeconds)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", rendered)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	exitCode := 0
	timedOut := false
	if err != nil {
		timedOut = ctx.Err() == context.DeadlineExceeded
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	s.saveAuditEvent(
		"function",
		name,
		"run",
		"system",
		actorIDForRequest(r),
		"",
		"global_function_run",
		nil,
		map[string]any{
			"name":            name,
			"exit_code":       exitCode,
			"timed_out":       timedOut,
			"input_bytes":     len(stdin),
			"timeout_seconds": int(timeout / time.Second),
			"env_keys":        appliedEnvKeys,
		},
	)

	status := http.StatusOK
	if err != nil {
		status = http.StatusBadGateway
	}
	s.respondJSON(w, status, map[string]any{
		"name":        name,
		"command":     fn.Command,
		"rendered":    rendered,
		"stdout":      strings.TrimSpace(stdout.String()),
		"stderr":      strings.TrimSpace(stderr.String()),
		"exit_code":   exitCode,
		"timed_out":   timedOut,
		"input_bytes": len(stdin),
		"env_keys":    appliedEnvKeys,
		"successful":  err == nil,
	})
}

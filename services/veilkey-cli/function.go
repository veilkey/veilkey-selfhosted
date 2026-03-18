package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	functionPlaceholderRe = regexp.MustCompile(`\{\%\{([A-Za-z_][A-Za-z0-9_]*)\}\%\}`)
	functionScopedRefRe   = regexp.MustCompile(`^(VK|VE):[A-Z_]+:.+$`)
	functionVaultHashRe   = regexp.MustCompile(`^[a-fA-F0-9]{8}$`)
)

var functionAllowlist = map[string]struct{}{
	"curl": {},
	"gh":   {},
	"git":  {},
	"glab": {},
}

type FunctionSpec struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	Command     string            `toml:"command"`
	Vars        map[string]string `toml:"vars"`
}

type functionRunOptions struct {
	VaultHash string
}

type functionSelector struct {
	Domain string
	Name   string
}

func resolveFunctionDir() string {
	if v := os.Getenv("VEILKEY_FUNCTION_DIR"); v != "" {
		return v
	}
	return ""
}

func functionFilePath(sel functionSelector) (string, error) {
	dir := resolveFunctionDir()
	if dir == "" {
		return "", errors.New("VEILKEY_FUNCTION_DIR is required for function commands")
	}
	if strings.TrimSpace(sel.Name) == "" {
		return "", errors.New("function name is required")
	}
	if strings.Contains(sel.Name, "/") || strings.Contains(sel.Name, string(filepath.Separator)) {
		return "", fmt.Errorf("invalid function name: %s", sel.Name)
	}
	if sel.Domain != "" {
		if strings.Contains(sel.Domain, "/") || strings.Contains(sel.Domain, string(filepath.Separator)) {
			return "", fmt.Errorf("invalid function domain: %s", sel.Domain)
		}
		return filepath.Join(dir, sel.Domain, sel.Name+".toml"), nil
	}
	return filepath.Join(dir, sel.Name+".toml"), nil
}

func loadFunctionSpec(sel functionSelector) (*FunctionSpec, error) {
	path, err := functionFilePath(sel)
	if err != nil {
		return nil, err
	}
	var spec FunctionSpec
	if _, err := toml.DecodeFile(path, &spec); err != nil {
		if sel.Domain != "" {
			legacyPath, legacyErr := functionFilePath(functionSelector{Name: sel.Domain + "-" + sel.Name})
			if legacyErr == nil {
				if _, legacyDecodeErr := toml.DecodeFile(legacyPath, &spec); legacyDecodeErr == nil {
					if spec.Name == "" {
						spec.Name = sel.Name
					}
					return &spec, validateFunctionSpec(&spec)
				}
			}
		}
		return nil, err
	}
	if spec.Name == "" {
		spec.Name = sel.Name
	}
	if err := validateFunctionSpec(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func validateFunctionSpec(spec *FunctionSpec) error {
	if spec == nil {
		return errors.New("function spec is nil")
	}
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("function name is required")
	}
	if strings.TrimSpace(spec.Command) == "" {
		return errors.New("command is required")
	}
	if len(spec.Vars) == 0 {
		return errors.New("at least one variable is required")
	}

	matches := functionPlaceholderRe.FindAllStringSubmatch(spec.Command, -1)
	seenPlaceholders := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		seenPlaceholders[match[1]] = struct{}{}
	}
	if len(seenPlaceholders) == 0 {
		return errors.New("command must contain at least one {%{NAME}%} placeholder")
	}

	for name, ref := range spec.Vars {
		if _, ok := seenPlaceholders[name]; !ok {
			return fmt.Errorf("vars.%s is not used by command placeholders", name)
		}
		if !functionScopedRefRe.MatchString(ref) {
			return fmt.Errorf("vars.%s must be a scoped VeilKey ref (VK:*:* or VE:*:*)", name)
		}
	}
	for name := range seenPlaceholders {
		if _, ok := spec.Vars[name]; !ok {
			return fmt.Errorf("placeholder %s has no vars.%s mapping", name, name)
		}
	}

	fields := strings.Fields(spec.Command)
	if len(fields) == 0 {
		return errors.New("command is empty")
	}
	if _, ok := functionAllowlist[fields[0]]; !ok {
		return fmt.Errorf("command %q is not in the allowlist", fields[0])
	}
	return nil
}

func renderFunctionCommand(spec *FunctionSpec, resolver func(string) (string, error)) (string, error) {
	if err := validateFunctionSpec(spec); err != nil {
		return "", err
	}
	var renderErr error
	output := functionPlaceholderRe.ReplaceAllStringFunc(spec.Command, func(token string) string {
		match := functionPlaceholderRe.FindStringSubmatch(token)
		if len(match) != 2 {
			renderErr = fmt.Errorf("invalid placeholder token: %s", token)
			return ""
		}
		ref := spec.Vars[match[1]]
		value, err := resolver(ref)
		if err != nil {
			renderErr = err
			return ""
		}
		return value
	})
	if renderErr != nil {
		return "", renderErr
	}
	return output, nil
}

func maskRenderedCommand(rendered string) string {
	return functionPlaceholderRe.ReplaceAllString(rendered, "{%{MASKED}%}")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func previewResolver(spec *FunctionSpec) func(string) (string, error) {
	return func(ref string) (string, error) {
		for key, candidate := range spec.Vars {
			if candidate == ref {
				return shellQuote(fmt.Sprintf("<masked:%s>", key)), nil
			}
		}
		return "", fmt.Errorf("unmapped ref in preview: %s", ref)
	}
}

func realResolver(client *VeilKeyClient) func(string) (string, error) {
	return func(ref string) (string, error) {
		value, err := client.Resolve(ref)
		if err != nil {
			return "", err
		}
		return shellQuote(value), nil
	}
}

func listFunctionNames() ([]string, error) {
	dir := resolveFunctionDir()
	if dir == "" {
		return nil, errors.New("VEILKEY_FUNCTION_DIR is required for function commands")
	}
	var names []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".toml") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".toml")
		names = append(names, rel)
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func encodeFunctionSpec(spec *FunctionSpec) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(spec); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeFunctionSpec(sel functionSelector, spec *FunctionSpec) error {
	if err := validateFunctionSpec(spec); err != nil {
		return err
	}
	path, err := functionFilePath(sel)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content, err := encodeFunctionSpec(spec)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func deleteFunctionSpec(sel functionSelector) error {
	path, err := functionFilePath(sel)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func currentFunctionVaultHash(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return strings.TrimSpace(os.Getenv("VEILKEY_CONTEXT_VAULT_HASH"))
}

func cmdFunctionTest(sel functionSelector, opts functionRunOptions) {
	spec, err := loadFunctionSpec(sel)
	if err != nil {
		fmt.Fprintf(errWriter, "ERROR: %v\n", err)
		os.Exit(1)
	}
	rendered, err := renderFunctionCommand(spec, previewResolver(spec))
	if err != nil {
		fmt.Fprintf(errWriter, "ERROR: %v\n", err)
		os.Exit(1)
	}
	if vaultHash := currentFunctionVaultHash(opts.VaultHash); vaultHash != "" {
		fmt.Fprintf(os.Stdout, "context_vault_hash=%s\n", vaultHash)
	}
	fmt.Fprintln(os.Stdout, rendered)
}

func cmdFunctionRun(sel functionSelector, opts functionRunOptions, apiURL string) {
	spec, err := loadFunctionSpec(sel)
	if err != nil {
		fmt.Fprintf(errWriter, "ERROR: %v\n", err)
		os.Exit(1)
	}
	client := NewVeilKeyClient(apiURL)
	rendered, err := renderFunctionCommand(spec, realResolver(client))
	if err != nil {
		fmt.Fprintf(errWriter, "ERROR: %v\n", err)
		os.Exit(1)
	}
	cmd := exec.Command("bash", "-lc", rendered)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if vaultHash := currentFunctionVaultHash(opts.VaultHash); vaultHash != "" {
		cmd.Env = append(cmd.Env, "VEILKEY_CONTEXT_VAULT_HASH="+vaultHash)
	}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(errWriter, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func parseFunctionSelector(args []string) (functionSelector, []string, error) {
	if len(args) == 0 {
		return functionSelector{}, nil, errors.New("function name is required")
	}
	if len(args) >= 2 && !functionVaultHashRe.MatchString(args[1]) {
		return functionSelector{Domain: args[0], Name: args[1]}, args[2:], nil
	}
	return functionSelector{Name: args[0]}, args[1:], nil
}

func cmdFunction(args []string, apiURL string) {
	if len(args) == 0 {
		fmt.Fprintln(errWriter, "Usage: veilkey-cli function <register|show|list|test|run|delete|domain name|name> ...")
		os.Exit(1)
	}
	switch args[0] {
	case "list":
		names, err := listFunctionNames()
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		for _, name := range names {
			fmt.Fprintln(os.Stdout, name)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(errWriter, "Usage: veilkey-cli function show <name> | <domain> <name>")
			os.Exit(1)
		}
		sel, _, err := parseFunctionSelector(args[1:])
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		spec, err := loadFunctionSpec(sel)
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		content, err := encodeFunctionSpec(spec)
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		os.Stdout.Write(content)
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(errWriter, "Usage: veilkey-cli function delete <name> | <domain> <name>")
			os.Exit(1)
		}
		sel, _, err := parseFunctionSelector(args[1:])
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		if err := deleteFunctionSpec(sel); err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
	case "register":
		cmdFunctionRegister(args[1:])
	case "test":
		if len(args) < 2 {
			fmt.Fprintln(errWriter, "Usage: veilkey-cli function test <name> [vault_hash] | <domain> <name> [vault_hash]")
			os.Exit(1)
		}
		sel, rest, err := parseFunctionSelector(args[1:])
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		opts := functionRunOptions{}
		if len(rest) >= 1 {
			opts.VaultHash = rest[0]
		}
		cmdFunctionTest(sel, opts)
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(errWriter, "Usage: veilkey-cli function run <name> [vault_hash] | <domain> <name> [vault_hash]")
			os.Exit(1)
		}
		sel, rest, err := parseFunctionSelector(args[1:])
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		opts := functionRunOptions{}
		if len(rest) >= 1 {
			opts.VaultHash = rest[0]
		}
		cmdFunctionRun(sel, opts, apiURL)
	default:
		sel, rest, err := parseFunctionSelector(args)
		if err != nil {
			fmt.Fprintf(errWriter, "ERROR: %v\n", err)
			os.Exit(1)
		}
		opts := functionRunOptions{}
		if len(rest) >= 1 {
			opts.VaultHash = rest[0]
		}
		cmdFunctionRun(sel, opts, apiURL)
	}
}

func cmdFunctionRegister(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(errWriter, "Usage: veilkey-cli function register <name> | <domain> <name> --description <text> --command <command> --var NAME=VK:*:*")
		os.Exit(1)
	}
	sel, rest, err := parseFunctionSelector(args)
	if err != nil {
		fmt.Fprintf(errWriter, "ERROR: %v\n", err)
		os.Exit(1)
	}
	spec := &FunctionSpec{
		Name: sel.Name,
		Vars: map[string]string{},
	}
	args = rest
	for len(args) > 0 {
		switch args[0] {
		case "--description":
			if len(args) < 2 {
				fmt.Fprintln(errWriter, "ERROR: --description requires a value")
				os.Exit(1)
			}
			spec.Description = args[1]
			args = args[2:]
		case "--command":
			if len(args) < 2 {
				fmt.Fprintln(errWriter, "ERROR: --command requires a value")
				os.Exit(1)
			}
			spec.Command = args[1]
			args = args[2:]
		case "--var":
			if len(args) < 2 {
				fmt.Fprintln(errWriter, "ERROR: --var requires NAME=REF")
				os.Exit(1)
			}
			parts := strings.SplitN(args[1], "=", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				fmt.Fprintf(errWriter, "ERROR: invalid --var %q\n", args[1])
				os.Exit(1)
			}
			spec.Vars[parts[0]] = parts[1]
			args = args[2:]
		default:
			fmt.Fprintf(errWriter, "ERROR: unknown function register option: %s\n", args[0])
			os.Exit(1)
		}
	}
	if err := writeFunctionSpec(sel, spec); err != nil {
		fmt.Fprintf(errWriter, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

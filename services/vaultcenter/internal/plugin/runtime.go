package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	extism "github.com/extism/go-sdk"
)

// HostFunctions are callbacks the plugin can invoke.
type HostFunctions struct {
	ResolveSecret func(name string) (string, bool)
	ResolveConfig func(name string) (string, bool)
}

// Instance is a loaded, running plugin backed by Extism.
type Instance struct {
	info    PluginInfo
	plugin  *extism.Plugin
	mu      sync.Mutex
	hostFns HostFunctions
}

// LoadInstance compiles and instantiates a WASM plugin via Extism.
func LoadInstance(ctx context.Context, wasmBytes []byte, hostFns HostFunctions) (*Instance, error) {
	inst := &Instance{hostFns: hostFns}

	// Host functions provided to the plugin
	hostResolveSecret := extism.NewHostFunctionWithStack(
		"host_resolve_secret",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			offset := stack[0]
			name, err := p.ReadString(offset)
			if err != nil {
				stack[0] = 0
				return
			}
			value, ok := hostFns.ResolveSecret(name)
			if !ok {
				stack[0] = 0
				return
			}
			out, err := p.WriteString(value)
			if err != nil {
				stack[0] = 0
				return
			}
			stack[0] = out
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
	hostResolveSecret.SetNamespace("veilkey")

	hostResolveConfig := extism.NewHostFunctionWithStack(
		"host_resolve_config",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			offset := stack[0]
			name, err := p.ReadString(offset)
			if err != nil {
				stack[0] = 0
				return
			}
			value, ok := hostFns.ResolveConfig(name)
			if !ok {
				stack[0] = 0
				return
			}
			out, err := p.WriteString(value)
			if err != nil {
				stack[0] = 0
				return
			}
			stack[0] = out
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
	hostResolveConfig.SetNamespace("veilkey")

	hostLog := extism.NewHostFunctionWithStack(
		"host_log",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			msg, _ := p.ReadString(stack[0])
			log.Printf("[plugin] %s", msg)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{},
	)
	hostLog.SetNamespace("veilkey")

	manifest := extism.Manifest{
		Wasm: []extism.Wasm{extism.WasmData{Data: wasmBytes}},
	}

	plugin, err := extism.NewPlugin(ctx, manifest, extism.PluginConfig{
		EnableWasi: true,
	}, []extism.HostFunction{hostResolveSecret, hostResolveConfig, hostLog})
	if err != nil {
		return nil, fmt.Errorf("create plugin: %w", err)
	}
	inst.plugin = plugin

	// Call plugin_info to get metadata
	info, err := inst.callJSON("plugin_info", nil)
	if err != nil {
		_ = plugin.Close(ctx)
		return nil, fmt.Errorf("plugin_info: %w", err)
	}
	var pInfo PluginInfo
	if err := json.Unmarshal(info, &pInfo); err != nil {
		_ = plugin.Close(ctx)
		return nil, fmt.Errorf("parse plugin_info: %w", err)
	}
	inst.info = pInfo

	return inst, nil
}

// Info returns the plugin's metadata.
func (inst *Instance) Info() PluginInfo {
	return inst.info
}

// Init calls plugin_init with the given config.
func (inst *Instance) Init(ctx context.Context, config map[string]any) (*PluginInitResult, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	input, _ := json.Marshal(config)
	out, err := inst.callJSON("plugin_init", input)
	if err != nil {
		return nil, err
	}
	var result PluginInitResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse init: %w", err)
	}
	return &result, nil
}

// Destroy calls plugin_destroy.
func (inst *Instance) Destroy(ctx context.Context) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	_, err := inst.callJSON("plugin_destroy", nil)
	return err
}

// Hooks calls plugin_hooks.
func (inst *Instance) Hooks(ctx context.Context) ([]HookDef, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	out, err := inst.callJSON("plugin_hooks", nil)
	if err != nil {
		return nil, err
	}
	var hooks []HookDef
	if err := json.Unmarshal(out, &hooks); err != nil {
		return nil, fmt.Errorf("parse hooks: %w", err)
	}
	return hooks, nil
}

// Paths calls plugin_paths.
func (inst *Instance) Paths(ctx context.Context) ([]string, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	out, err := inst.callJSON("plugin_paths", nil)
	if err != nil {
		return nil, err
	}
	var paths []string
	if err := json.Unmarshal(out, &paths); err != nil {
		return nil, fmt.Errorf("parse paths: %w", err)
	}
	return paths, nil
}

// Validate calls plugin_validate.
func (inst *Instance) Validate(ctx context.Context, path, content string) (*ValidateResult, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	input, _ := json.Marshal(map[string]string{"path": path, "content": content})
	out, err := inst.callJSON("plugin_validate", input)
	if err != nil {
		return nil, err
	}
	var result ValidateResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse validate: %w", err)
	}
	return &result, nil
}

// Render calls plugin_render.
func (inst *Instance) Render(ctx context.Context, action string, input any) (*RenderResult, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	req := RenderRequest{Action: action, Input: input}
	data, _ := json.Marshal(req)
	out, err := inst.callJSON("plugin_render", data)
	if err != nil {
		return nil, err
	}
	var result RenderResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse render: %w", err)
	}
	return &result, nil
}

// Close releases the Extism plugin.
func (inst *Instance) Close(ctx context.Context) error {
	_ = inst.plugin.Close(ctx)
	return nil
}

// callJSON calls a plugin function with JSON input and returns JSON output.
func (inst *Instance) callJSON(fnName string, input []byte) ([]byte, error) {
	if input == nil {
		input = []byte("{}")
	}
	exit, out, err := inst.plugin.Call(fnName, input)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fnName, err)
	}
	if exit != 0 {
		return nil, fmt.Errorf("%s: exit code %d", fnName, exit)
	}
	return out, nil
}

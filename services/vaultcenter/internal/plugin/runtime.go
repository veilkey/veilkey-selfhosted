package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// HostFunctions are callbacks the plugin can invoke.
type HostFunctions struct {
	ResolveSecret func(name string) (string, bool)
	ResolveConfig func(name string) (string, bool)
}

// Instance is a loaded, running plugin.
type Instance struct {
	info    PluginInfo
	rt      wazero.Runtime
	mod     api.Module
	mu      sync.Mutex
	hostFns HostFunctions
}

// LoadInstance compiles and instantiates a WASM plugin.
func LoadInstance(ctx context.Context, wasmBytes []byte, hostFns HostFunctions) (*Instance, error) {
	rt := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	inst := &Instance{rt: rt, hostFns: hostFns}

	_, err := rt.NewHostModuleBuilder("veilkey").
		NewFunctionBuilder().WithFunc(inst.hostLog).Export("host_log").
		NewFunctionBuilder().WithFunc(inst.hostResolveSecret).Export("host_resolve_secret").
		NewFunctionBuilder().WithFunc(inst.hostResolveConfig).Export("host_resolve_config").
		Instantiate(ctx)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("register host module: %w", err)
	}

	mod, err := rt.Instantiate(ctx, wasmBytes)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("instantiate wasm: %w", err)
	}
	inst.mod = mod

	info, err := inst.callPluginInfo(ctx)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("plugin_info: %w", err)
	}
	inst.info = *info
	return inst, nil
}

func (inst *Instance) Info() PluginInfo { return inst.info }

func (inst *Instance) Init(ctx context.Context, config map[string]any) (*PluginInitResult, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	input, _ := json.Marshal(config)
	out, err := inst.callGuest(ctx, "plugin_init", input)
	if err != nil { return nil, err }
	var result PluginInitResult
	if err := json.Unmarshal(out, &result); err != nil { return nil, fmt.Errorf("parse init result: %w", err) }
	return &result, nil
}

func (inst *Instance) Destroy(ctx context.Context) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	_, err := inst.callGuest(ctx, "plugin_destroy", nil)
	return err
}

func (inst *Instance) Hooks(ctx context.Context) ([]HookDef, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	out, err := inst.callGuest(ctx, "plugin_hooks", nil)
	if err != nil { return nil, err }
	var hooks []HookDef
	if err := json.Unmarshal(out, &hooks); err != nil { return nil, fmt.Errorf("parse hooks: %w", err) }
	return hooks, nil
}

func (inst *Instance) Paths(ctx context.Context) ([]string, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	out, err := inst.callGuest(ctx, "plugin_paths", nil)
	if err != nil { return nil, err }
	var paths []string
	if err := json.Unmarshal(out, &paths); err != nil { return nil, fmt.Errorf("parse paths: %w", err) }
	return paths, nil
}

func (inst *Instance) Validate(ctx context.Context, path, content string) (*ValidateResult, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	input, _ := json.Marshal(map[string]string{"path": path, "content": content})
	out, err := inst.callGuest(ctx, "plugin_validate", input)
	if err != nil { return nil, err }
	var result ValidateResult
	if err := json.Unmarshal(out, &result); err != nil { return nil, fmt.Errorf("parse validate: %w", err) }
	return &result, nil
}

func (inst *Instance) Render(ctx context.Context, action string, input any) (*RenderResult, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	req := RenderRequest{Action: action, Input: input}
	data, _ := json.Marshal(req)
	out, err := inst.callGuest(ctx, "plugin_render", data)
	if err != nil { return nil, err }
	var result RenderResult
	if err := json.Unmarshal(out, &result); err != nil { return nil, fmt.Errorf("parse render: %w", err) }
	return &result, nil
}

func (inst *Instance) Close(ctx context.Context) error { return inst.rt.Close(ctx) }

func (inst *Instance) callPluginInfo(ctx context.Context) (*PluginInfo, error) {
	out, err := inst.callGuest(ctx, "plugin_info", nil)
	if err != nil { return nil, err }
	var info PluginInfo
	if err := json.Unmarshal(out, &info); err != nil { return nil, fmt.Errorf("parse plugin_info: %w", err) }
	return &info, nil
}

func (inst *Instance) callGuest(ctx context.Context, fnName string, input []byte) ([]byte, error) {
	fn := inst.mod.ExportedFunction(fnName)
	if fn == nil { return nil, fmt.Errorf("plugin does not export %s", fnName) }
	if input == nil { input = []byte("{}") }

	malloc := inst.mod.ExportedFunction("malloc")
	if malloc == nil { return nil, fmt.Errorf("plugin does not export malloc") }

	inputLen := uint64(len(input))
	results, err := malloc.Call(ctx, inputLen)
	if err != nil { return nil, fmt.Errorf("malloc(%d): %w", inputLen, err) }
	inputPtr := results[0]

	if !inst.mod.Memory().Write(uint32(inputPtr), input) {
		return nil, fmt.Errorf("memory write failed at ptr=%d len=%d", inputPtr, inputLen)
	}

	ret, err := fn.Call(ctx, inputPtr, inputLen)
	if err != nil { return nil, fmt.Errorf("%s call: %w", fnName, err) }
	if len(ret) == 0 { return []byte("{}"), nil }

	packed := ret[0]
	resultPtr := uint32(packed >> 32)
	resultLen := uint32(packed & 0xFFFFFFFF)
	if resultLen == 0 { return []byte("{}"), nil }

	out, ok := inst.mod.Memory().Read(resultPtr, resultLen)
	if !ok { return nil, fmt.Errorf("memory read failed at ptr=%d len=%d", resultPtr, resultLen) }

	if free := inst.mod.ExportedFunction("free"); free != nil {
		_, _ = free.Call(ctx, uint64(resultPtr), uint64(resultLen))
	}
	return out, nil
}

func (inst *Instance) hostLog(ctx context.Context, m api.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
	level, _ := m.Memory().Read(levelPtr, levelLen)
	msg, _ := m.Memory().Read(msgPtr, msgLen)
	log.Printf("[plugin:%s] [%s] %s", inst.info.Name, string(level), string(msg))
}

func (inst *Instance) hostResolveSecret(ctx context.Context, m api.Module, namePtr, nameLen uint32) uint64 {
	name, _ := m.Memory().Read(namePtr, nameLen)
	value, ok := inst.hostFns.ResolveSecret(string(name))
	if !ok { return 0 }
	return inst.writeToGuest(ctx, []byte(value))
}

func (inst *Instance) hostResolveConfig(ctx context.Context, m api.Module, namePtr, nameLen uint32) uint64 {
	name, _ := m.Memory().Read(namePtr, nameLen)
	value, ok := inst.hostFns.ResolveConfig(string(name))
	if !ok { return 0 }
	return inst.writeToGuest(ctx, []byte(value))
}

func (inst *Instance) writeToGuest(ctx context.Context, data []byte) uint64 {
	malloc := inst.mod.ExportedFunction("malloc")
	if malloc == nil { return 0 }
	results, err := malloc.Call(ctx, uint64(len(data)))
	if err != nil { return 0 }
	ptr := uint32(results[0])
	if !inst.mod.Memory().Write(ptr, data) { return 0 }
	return (uint64(ptr) << 32) | uint64(len(data))
}

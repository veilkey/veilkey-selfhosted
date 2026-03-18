package hkm

// HkmRuntimeInfo returns runtime node information as a JSON-serialisable map.
// It is the exported entry point used by api.Server.handleStatus.
func (h *Handler) HkmRuntimeInfo() (map[string]interface{}, error) {
	return h.hkmRuntimeInfo()
}

func (h *Handler) hkmRuntimeInfo() (map[string]interface{}, error) {
	info, err := h.deps.DB().GetNodeInfo()
	if err != nil {
		return nil, err
	}

	childCount, _ := h.deps.DB().CountChildren()
	trackedRefCount, _ := h.deps.DB().CountRefs()
	secretCount, _ := h.deps.DB().CountSecrets()
	configCount, _ := h.deps.DB().CountConfigs()

	resp := map[string]interface{}{
		"mode":               "hkm",
		"node_id":            info.NodeID,
		"vault_node_uuid":    info.NodeID,
		"version":            info.Version,
		"children_count":     childCount,
		"tracked_refs_count": trackedRefCount,
		"secrets_count":      secretCount,
		"configs_count":      configCount,
	}
	if info.ParentURL != "" {
		resp["parent_url"] = info.ParentURL
	}
	return resp, nil
}

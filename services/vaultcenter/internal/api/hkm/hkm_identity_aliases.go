package hkm

func setNodeIdentityAliases(data map[string]interface{}, nodeID string) {
	if nodeID == "" {
		return
	}
	data["vault_node_uuid"] = nodeID
	data["node_id"] = nodeID
}

func setRuntimeHashAliases(data map[string]interface{}, agentHash string) {
	if agentHash == "" {
		return
	}
	data["vault_runtime_hash"] = agentHash
	data["agent_hash"] = agentHash
}

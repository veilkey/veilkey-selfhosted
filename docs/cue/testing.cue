package facts

testing: {
	local_commands: {
		installer: [
			"./install.sh init",
			"./install.sh validate",
			"./install.sh doctor",
			"bash tests/test_proxy_component.sh",
			"bash tests/test_install_profile_command.sh",
			"bash tests/test_proxmox_wrapper_commands.sh",
		]
		keycenter: [
			"go test ./cmd/... ./internal/api/... ./internal/db/...",
		]
		localvault: [
			"go test ./cmd/... ./internal/api/... ./internal/db/...",
		]
		proxy: [
			"go test ./...",
			"bash tests/run-shell-tests.sh",
		]
		cli: [
			"go test ./...",
			"bash tests/test_session_config.sh",
			"bash tests/test_install_user_boundary.sh",
			"bash tests/test_veilroot_session.sh",
		]
	}
}

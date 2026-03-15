package facts

#Job: string & =~"^[a-z0-9][a-z0-9-]*$"

ci: {
	validate_jobs: [...#Job] & [
		"facts-validate",
		"installer-validate",
		"keycenter-validate",
		"localvault-validate",
		"cli-validate",
		"proxy-validate",
	]
	e2e_jobs: [...#Job] & [
		"selfhosted-e2e-proxmox-allinone",
		"selfhosted-e2e-proxmox-runtime",
		"selfhosted-e2e-proxmox-account-smoke",
	]
}

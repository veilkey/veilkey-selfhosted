package facts

endpoints: {
	forbidden_literals: [
		"10180",
		"10.50.100.210",
		"gitlab.ranode.net",
	]
	forbidden_scan_paths: [
		"README.md",
		"docs",
		"services/keycenter/README.md",
		"services/localvault/README.md",
	]
	forbidden_skip_paths: [
		"docs/cue",
		"docs/evidence",
	]
}

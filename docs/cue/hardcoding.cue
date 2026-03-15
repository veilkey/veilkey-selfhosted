package facts

hardcoding: {
	env_specific_regexes: [
		"10\\.50\\.[0-9]{1,3}\\.[0-9]{1,3}",
		"10\\.60\\.[0-9]{1,3}\\.[0-9]{1,3}",
		"gitlab\\.ranode\\.net",
		"keycenter\\.60\\.internal\\.kr",
		"gitlab\\.60\\.internal\\.kr",
		"mattermost\\.50\\.internal\\.kr",
	]
	env_specific_scan_paths: [
		"README.md",
		".gitlab-ci.yml",
		"docs",
		"services",
		"client",
		"installer",
	]
	env_specific_skip_paths: [
		".git",
		".tmp",
		"docs/cue",
		"docs/generated",
		"docs/evidence",
		"installer/validation-logs",
	]
	env_specific_baseline_file: "docs/evidence/hardcoding-env-specific-baseline.txt"
	report_scan_paths: [
		"README.md",
		".gitlab-ci.yml",
		"docs",
		"services",
		"client",
		"installer",
	]
	report_skip_paths: [
		".git",
		".tmp",
		"docs/cue",
		"docs/generated",
		"docs/evidence",
		"installer/validation-logs",
	]
	report_regexes: [
		"10\\.50\\.[0-9]{1,3}\\.[0-9]{1,3}",
		"10\\.60\\.[0-9]{1,3}\\.[0-9]{1,3}",
		"10\\.50\\.100\\.210",
		"10\\.60\\.100\\.210",
		"127\\.0\\.0\\.1",
		"localhost",
		"gitlab\\.ranode\\.net",
		"keycenter\\.60\\.internal\\.kr",
		"gitlab\\.60\\.internal\\.kr",
		"10180",
		"10181",
		"18080",
		"18081",
		"18083",
		"18084",
	]
	enforced_literals: [
		"10.50.100.210",
		"10.60.100.210",
		"gitlab.ranode.net",
	]
	enforced_scan_paths: [
		"README.md",
		"docs",
		"services/keycenter/README.md",
		"services/localvault/README.md",
	]
	enforced_skip_paths: [
		"docs/cue",
		"docs/generated",
		"docs/evidence",
	]
}

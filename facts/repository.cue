package facts

repo: {
	name:        "veilkey-selfhosted"
	domain:      "self-hosted"
	root_readme: "README.md"
	facts_dir:   "facts"
	version_files: {
		current: "VERSION"
		policy:  "VERSIONING.md"
	}
	canonical_docs: {
		root:       "README.md"
		localvault: "services/localvault/README.md"
	}
	paths: {
		vaultcenter: "services/vaultcenter"
		localvault:  "services/localvault"
	}
}

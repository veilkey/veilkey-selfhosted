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
		root:      "README.md"
		vaultcenter: "services/vaultcenter/README.md"
		localvault: "services/localvault/README.md"
	}
	paths: {
		installer:  "installer"
		vaultcenter:  "services/vaultcenter"
		localvault: "services/localvault"
		proxy:      "services/proxy"
		cli:        "client/cli"
	}
}

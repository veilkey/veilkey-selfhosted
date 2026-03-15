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
		keycenter: "services/keycenter/README.md"
		localvault: "services/localvault/README.md"
	}
	paths: {
		installer:  "installer"
		keycenter:  "services/keycenter"
		localvault: "services/localvault"
		proxy:      "services/proxy"
		cli:        "client/cli"
	}
}

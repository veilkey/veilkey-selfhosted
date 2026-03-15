package facts

repo: {
	name:          "veilkey-selfhosted"
	domain:        "self-hosted"
	product_version: "0.1.0"
	root_readme:   "README.md"
	facts_dir:     "docs/cue"
	version_files: {
		current: "VERSION"
		policy:  "VERSIONING.md"
	}
	canonical_docs: {
		root:       "README.md"
		keycenter:  "services/keycenter/README.md"
		localvault: "services/localvault/README.md"
	}
	paths: {
		installer:  "installer"
		keycenter:  "services/keycenter"
		localvault: "services/localvault"
		proxy:      "services/proxy"
		cli:        "client/cli"
		docs:       "docs"
	}
}

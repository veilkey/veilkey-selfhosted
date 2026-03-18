package docs

// selfhosted.cue
// CUE schema that validates the veilkey-selfhosted monorepo structure.
// Complements docs_contract.cue with path and version-file constraints
// derived from facts/repository.cue.

// ---------------------------------------------------------------------------
// Component paths constraint
// ---------------------------------------------------------------------------
// Every entry must be a non-empty relative path (no leading slash).

#ComponentPaths: {
	installer:  =~"^[^/].*"
	keycenter:  =~"^services/"
	localvault: =~"^services/"
	proxy:      =~"^services/"
	cli:        =~"^client/"
}

component_paths: #ComponentPaths & {
	installer:  "installer"
	keycenter:  "services/keycenter"
	localvault: "services/localvault"
	proxy:      "services/proxy"
	cli:        "client/cli"
}

// ---------------------------------------------------------------------------
// Version file constraint
// ---------------------------------------------------------------------------
// The repo must declare both a current version file and a versioning policy.

#VersionFiles: {
	current: string & =~"^[A-Z]+"
	policy:  string & =~"\\.md$"
}

version_files: #VersionFiles & {
	current: "VERSION"
	policy:  "VERSIONING.md"
}

// ---------------------------------------------------------------------------
// Required README paths
// ---------------------------------------------------------------------------
// Each component that ships documentation must have a README at its path.

#RequiredREADMEs: {
	root:      "README.md"
	keycenter: =~"^services/keycenter/.+\\.md$"
	localvault: =~"^services/localvault/.+\\.md$"
}

required_readmes: #RequiredREADMEs & {
	root:       "README.md"
	keycenter:  "services/keycenter/README.md"
	localvault: "services/localvault/README.md"
}

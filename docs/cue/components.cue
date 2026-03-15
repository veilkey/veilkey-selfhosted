package facts

#Path: string & !=""
#LocalCIJob: string & =~"^[A-Za-z0-9][A-Za-z0-9:_-]*$"

#Component: {
	kind:          "installer" | "service" | "client"
	path:          #Path
	readme:        #Path
	local_ci_file: #Path
	local_ci_jobs: [...#LocalCIJob]
}

components: {
	names: ["installer", "keycenter", "localvault", "proxy", "cli"]
	installer: #Component & {
		kind:          "installer"
		path:          "installer"
		readme:        "installer/README.md"
		local_ci_file: "installer/.gitlab-ci.yml"
		local_ci_jobs: [
			"validate-components",
			"validate-install-layout",
			"export-component-manifest",
			"mr-guard",
			"rulebook-format",
		]
	}
	keycenter: #Component & {
		kind:          "service"
		path:          "services/keycenter"
		readme:        "services/keycenter/README.md"
		local_ci_file: "services/keycenter/.gitlab-ci.yml"
		local_ci_jobs: [
			"test",
			"build",
			"package",
			"mr-guard",
		]
	}
	localvault: #Component & {
		kind:          "service"
		path:          "services/localvault"
		readme:        "services/localvault/README.md"
		local_ci_file: "services/localvault/.gitlab-ci.yml"
		local_ci_jobs: [
			"test",
			"build",
			"package",
			"mr-guard",
		]
	}
	proxy: #Component & {
		kind:          "service"
		path:          "services/proxy"
		readme:        "services/proxy/README.md"
		local_ci_file: "services/proxy/.gitlab-ci.yml"
		local_ci_jobs: [
			"go:test",
			"go:build",
		]
	}
	cli: #Component & {
		kind:          "client"
		path:          "client/cli"
		readme:        "client/cli/README.md"
		local_ci_file: "client/cli/.gitlab-ci.yml"
		local_ci_jobs: [
			"lint",
			"test",
			"build",
			"release",
		]
	}
}

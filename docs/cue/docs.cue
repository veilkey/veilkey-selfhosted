package facts

#Path: string & !=""

docs: {
	primary_entrypoint:     "README.md"
	repository_docs_hub:    "docs/README.md"
	service_readmes: [...#Path] & [
		"services/keycenter/README.md",
		"services/localvault/README.md",
	]
	service_readme_root_link: "[`../../README.md`](../../README.md)"
	canonical_facts_dir:     "docs/cue/"
	root_sections: [
		"## Repository Position",
		"## Repository Layout",
		"## Runtime Model",
		"## Responsibility Boundary",
		"## Validation and CI",
		"## Local Test Entry Points",
		"## Service Docs",
	]
	root_required_strings: [
		repo.name,
		canonical_facts_dir,
		"docs/README.md",
		"docs/architecture.md",
		"docs/testing.md",
		"docs/runtime-contracts.md",
		"docs/cue/repository.cue",
		root_sections[0],
		root_sections[1],
		root_sections[2],
		root_sections[3],
		root_sections[4],
		root_sections[5],
		root_sections[6],
	]
	service_readme_required_strings: {
		keycenter: [
			repo.name,
			service_readme_root_link,
			identity.primary_terms[0],
			identity.primary_terms[1],
			identity.primary_terms[2],
		]
		localvault: [
			repo.name,
			service_readme_root_link,
			identity.primary_terms[0],
			identity.primary_terms[1],
			identity.primary_terms[2],
		]
	}
}

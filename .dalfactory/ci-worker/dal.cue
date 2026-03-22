schema_version: "1.0.0"

dal: {
	name:   "ci-worker"
	role:   "CI/빌드 워커 — 빌드, 테스트, 린트 실행"
	player: "codex"
}

container: {
	base:     "ubuntu:24.04"
	memory:   2048
	cores:    2
	packages: ["golang-go", "nodejs", "npm", "make", "bats"]
}

schema_version: "1.0.0"

dal: {
	name:   "tech-writer"
	role:   "기술 콘텐츠 담당 — 아키텍처 분석, 문서 작성, 코드 리뷰"
	player: "claude"
}

container: {
	base:     "ubuntu:24.04"
	memory:   4096
	cores:    4
	packages: ["golang-go", "nodejs", "npm", "python3", "make"]
}

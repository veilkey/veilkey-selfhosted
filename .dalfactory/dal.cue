schema_version: "1.0.0"

dal: {
	id:       "DAL:REPO:veilkey-selfhosted"
	name:     "veilkey-selfhosted"
	version:  "0.1.0"
	category: "REPO"
}

description: "VeilKey self-hosted secret manager"

templates: default: {
	schema_version: "1.0.0"
	name:           "default"
	description:    "VeilKey development and discussion"
	container: {
		base:     "ubuntu:24.04"
		packages: ["bash", "git", "curl", "golang-go"]
		agents: {}
	}
	build: {
		language: "go"
	}
	exports: claude: {
		skills: []
	}
}

talk: {
	channel:   "veilkey"
	conductor: "keycenter"
	agents: [
		{username: "agent-200", role: "마케팅 전략가 — 포지셔닝, 슬로건, 채널 전략"},
		{username: "agent-tech-201", role: "기술 콘텐츠 담당 — 아키텍처, 문서, 개발자 관점"},
	]
}

schema_version: "1.0.0"

dal: {
	name:   "marketing"
	role:   "마케팅 전략가 — 포지셔닝, 슬로건, 채널 전략"
	player: "claude"
}

container: {
	base:     "ubuntu:24.04"
	memory:   2048
	cores:    2
	packages: ["nodejs", "npm"]
}

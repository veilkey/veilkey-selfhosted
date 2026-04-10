uuid:    "vk-verifier-20260410"
name:    "verifier"
version: "1.1.0"
player:  "claude"
role:    "member"
model:   "sonnet"
git: {
	user:  "dal-verifier"
	email: "dal-verifier@veilkey.local"
}

// Read 전용 — Edit/Write/Bash 절대 없음. 검증만.
tools: ["Read", "Glob", "Grep"]

dalcli: {
	own: {
		commands: [] // 커맨드 실행 권한 0 — 정말 Read만
	}
}

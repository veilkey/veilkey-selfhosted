uuid:    "vk-dev-20260410"
name:    "dev"
version: "1.1.0"
player:  "claude"
role:    "member"
model:   "sonnet"
git: {
	user:  "dal-dev"
	email: "dal-dev@veilkey.local"
}

// claude SDK 도구 — 파일 수정 + Bash (cargo/npm 호출)
tools: ["Read", "Write", "Edit", "Glob", "Grep", "Bash"]

dalcli: {
	own: {
		commands: [
			"git status",
			"git diff",
			"git add *",
			"git commit -m *",
			"cargo build *",
			"cargo test *",
			"cargo check *",
			"npm install",
			"npm run *",
		]
	}
}

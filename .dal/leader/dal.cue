uuid:    "vk-leader-20260410"
name:    "leader"
version: "1.1.0"
player:  "claude"
role:    "leader"
model:   "opus"
git: {
	user:  "dal-leader"
	email: "dal-leader@veilkey.local"
}

// claude SDK 도구 — Read 전용 (수정/배포는 모두 위임)
tools: ["Read", "Glob", "Grep"]

dalcli: {
	own: {
		commands: ["git status", "git log --oneline -*", "ls *", "cat *"]
	}
	can_delegate: {
		commands: ["git push *", "cargo *", "npm *"]
		to: ["dev", "verifier", "tech-writer", "ci-worker", "marketing"]
	}
}

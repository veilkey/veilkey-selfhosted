// dal.spec.cue — localdal schema

#Player: "claude" | "codex" | "gemini"
#Role:   "leader" | "member" | "ops"

#BranchConfig: {
	base?: string | *"main"
}

#SetupConfig: {
	packages?: [...string]
	commands?: [...string]
	timeout?:  string | *"5m"
}

#DalProfile: {
	uuid!:           string & != ""
	name!:           string & != ""
	version!:        string
	player!:           #Player
	fallback_player?:  #Player
	role!:             #Role
	channel_only?:   bool
	skills?:         [...string]
	hooks?:          [...string]
	model?:          string
	player_version?: string
	auto_task?:      string
	auto_interval?:  string
	workspace?:      string
	branch?:         #BranchConfig
	setup?:          #SetupConfig
	git?: {
		user?:         string
		email?:        string
		github_token?: string
	}
}

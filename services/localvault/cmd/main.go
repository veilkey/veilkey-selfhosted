package main

import (
	"os"

	"veilkey-localvault/internal/commands"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			commands.RunInit()
			return
		case "cron":
			commands.RunCron()
			return
		case "rebind":
			commands.RunRebind()
			return
		}
	}
	commands.RunServer()
}

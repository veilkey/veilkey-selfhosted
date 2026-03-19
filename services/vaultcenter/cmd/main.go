package main

import (
	"os"

	"veilkey-vaultcenter/internal/commands"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			commands.RunInit()
			return
		}
	}

	commands.RunServer()
}

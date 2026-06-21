package main

import (
	"mework/libs/client/cli"

	// Blank-import sandbox engine drivers.
	_ "mework/libs/sandbox/engine/local"
)

func main() {
	cli.Execute()
}

// Command mework-sandbox is the standalone sandbox daemon. It starts a gRPC/HTTP
// listener and waits for runner connections to sandboxed agent execution.
//
// This is a wiring stub — the actual listener and engine selection land in
// the sandbox-runtime change. For now it starts, logs, and blocks.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "mework/libs/sandbox/engine/local"
)

func main() {
	fmt.Println("mework-sandbox: starting (stub mode)")
	fmt.Println("mework-sandbox: wired engines: local")

	// Block until SIGINT/SIGTERM.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("mework-sandbox: shutting down")
}

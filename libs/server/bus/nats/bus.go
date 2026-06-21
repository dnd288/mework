// Package nats implements the Bus interface using NATS JetStream for
// durable, distributed message delivery.
//
// This is a stub — the full implementation lands in a downstream change.
// The NATS SDK dependency lives only in this subpackage.
package nats

import "fmt"

func init() {
	fmt.Println("nats bus stub registered")
}

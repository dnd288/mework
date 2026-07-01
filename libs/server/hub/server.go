package hub

import (
	mezonadapter "mework/libs/server/provider/mezon"
)

// init registers the Mezon adapter with the global provider registry.
// The adapter is registered without a bot; write-back to Mezon is handled
// by the standalone worker binary, not by the server.
func init() {
	mezonadapter.RegisterAdapter()
}

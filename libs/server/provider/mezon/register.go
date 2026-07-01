package mezon

import "mework/libs/server/provider"

// RegisterAdapter registers the MezonAdapter with the global provider registry.
// The adapter is registered without a bot reference; write-back is handled
// by the standalone worker.
func RegisterAdapter() {
	provider.Register(NewMezonAdapter(nil))
}

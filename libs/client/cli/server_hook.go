package cli

import "context"

// ServerStarter boots the hub in-process. It is implemented in the apps/mework
// binary (the root module, which may import libs/server) and injected via
// SetServerStarter. listen is the resolved listen address ("" means "use the
// configured/default address").
type ServerStarter func(ctx context.Context, listen string) error

// serverStartFn holds the injected in-process hub starter. It is nil in builds
// that do not wire one (e.g. the libs/client module's own tests), in which case
// `server start` fails with a clear, actionable error.
var serverStartFn ServerStarter

// SetServerStarter wires the in-process hub starter used by `server start`.
// The apps/mework binary calls this at startup; the libs/client module cannot
// import libs/server directly (it would pull pgx/chi/goose into the client),
// so the dependency lives behind this seam — mirroring SetSessionResolverFactory.
func SetServerStarter(fn ServerStarter) {
	serverStartFn = fn
}

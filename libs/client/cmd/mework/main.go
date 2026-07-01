package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"mework/libs/client/cli"
	"mework/libs/sandbox"
	"mework/libs/shared/core"

	// Blank-import sandbox engine drivers.
	_ "mework/libs/sandbox/engine/local"
)

func main() {
	cli.Execute()
}

// catalogLister is the read-only view of the agent catalog the `agent list`
// command needs: it enumerates the available prebuilt definitions.
type catalogLister interface {
	ListDefinitions(ctx context.Context) ([]sandbox.SandboxBundleMetadata, error)
}

// sessionLister is the read-only view of the session client the `session list`
// command needs: it enumerates a tenant's sessions.
type sessionLister interface {
	ListSessions(ctx context.Context, tenant core.TenantID) ([]core.SessionInfo, error)
}

// RenderAgentList writes a read-only table of the prebuilt definitions known to
// the catalog. It never mutates the catalog.
func RenderAgentList(ctx context.Context, w io.Writer, cat catalogLister) error {
	defs, err := cat.ListDefinitions(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tVERSION\tENGINE\tBACKEND\tIMAGE")
	for _, d := range defs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.Name, d.Version, d.Engine, d.Backend, d.Image)
	}
	return tw.Flush()
}

// RenderSessionList writes a read-only table of the caller tenant's sessions.
// Sessions are scoped to the caller's tenant by the session client; this command
// never mutates session state.
func RenderSessionList(ctx context.Context, w io.Writer, sc sessionLister, tenant core.TenantID) error {
	sessions, err := sc.ListSessions(ctx, tenant)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tOWNER")
	for _, s := range sessions {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ID, s.Status, s.Owner)
	}
	return tw.Flush()
}

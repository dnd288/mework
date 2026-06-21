package channel

import (
	"context"
	"fmt"
	"log"
	"time"

	"mework/libs/server/bus"
	"mework/libs/server/catalog"
	"mework/libs/server/registry"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
)

// AutoProvisioner selects an eligible worker, creates a session, binds the
// channel, and dispatches the agent when no session exists for a channel.
type AutoProvisioner struct {
	registrySvc  *registry.Service
	channelReg   Registry
	sessionMgr   *session.Manager
	catalog      *catalog.AgentHandlers
	broker       bus.Broker
	tenantID     string
}

// NewAutoProvisioner creates a new AutoProvisioner with the given tenant ID.
// The tenant ID is used for worker selection scoping.
func NewAutoProvisioner(svc *registry.Service, channelReg Registry, sessionMgr *session.Manager, agentHandlers *catalog.AgentHandlers, broker bus.Broker, tenantID string) *AutoProvisioner {
	return &AutoProvisioner{
		registrySvc: svc,
		channelReg:  channelReg,
		sessionMgr:  sessionMgr,
		catalog:     agentHandlers,
		broker:      broker,
		tenantID:    tenantID,
	}
}

// Provision selects a worker matching the spec, creates a session, binds the
// channel, dispatches the agent, and returns the session ID. Retries up to 3
// times with 5s backoff when no eligible worker is found.
func (p *AutoProvisioner) Provision(ctx context.Context, providerCode, resourceID, spec string) (string, error) {
	channelKey := providerCode + ":" + resourceID

	// Select a worker matching the spec
	runner, err := p.selectWorkerWithRetry(ctx, spec)
	if err != nil {
		return "", err
	}

	// Create session
	agentName := spec
	if agentName == "" {
		agentName = "default"
	}
	sInfo, err := p.sessionMgr.Create(ctx, agentName, "latest", runner.ID, core.AccountID(runner.AccountID), core.TenantID(runner.TenantID))
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	// Bind channel
	err = p.channelReg.Bind(ctx, channelKey, string(sInfo.ID), runner.ID, providerCode, resourceID, spec)
	if err != nil {
		return "", fmt.Errorf("bind channel: %w", err)
	}

	// Dispatch agent with a pull+spawn grant
	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpSpawn}, nil)
	if err != nil {
		return "", fmt.Errorf("create grant: %w", err)
	}

	err = p.catalog.DispatchToRunner(ctx, agentName, runner.ID, g)
	if err != nil {
		// Log but don't fail — the session and binding are already created
		log.Printf("dispatch agent to runner %s: %v", runner.ID, err)
	}

	return string(sInfo.ID), nil
}

// selectWorkerWithRetry attempts to select a worker matching the spec, retrying
// up to 3 times with 5s backoff between attempts.
func (p *AutoProvisioner) selectWorkerWithRetry(ctx context.Context, spec string) (*registry.Runtime, error) {
	tenant := registry.Tenant{ID: p.tenantID}

	for i := 0; i < 3; i++ {
		runner, err := p.registrySvc.SelectWorker(ctx, spec, tenant)
		if err != nil {
			if i < 2 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(5 * time.Second):
				}
				continue
			}
			return nil, err
		}
		return runner, nil
	}

	return nil, fmt.Errorf("no eligible worker found after retries")
}

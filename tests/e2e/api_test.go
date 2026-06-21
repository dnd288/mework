package e2e

import (
	"context"
	"time"
)

// ============================================================================
// PROPOSED API (review surface)
//
// These types and interfaces are the agent-hub API the scenarios drive. They live
// only in the test build (no production code). Reviewing them — together with how the
// scenarios use them — is the point: evaluate the shapes and behaviors before building.
// When implementation starts, these lift into internal/shared/transport + the capability
// packages (openspec c0002 layout) and the World methods get real bodies.
// ============================================================================

// ---- Identity & tenancy -----------------------------------------------------

type (
	TenantID  string
	AccountID string
	RunnerID  string
	SessionID string
	SandboxID string
)

type Tenant struct {
	ID   TenantID
	Name string
}

// Identity is the authenticated principal behind a request (a user via PAT, or a runner
// via its durable credential), always scoped to a tenant.
type Identity struct {
	Account AccountID
	Runner  RunnerID
	Tenant  TenantID
}

// RunnerIdentity is the durable credential persisted after enrollment (0600). It is NOT
// the short-lived registration token used to obtain it.
type RunnerIdentity struct {
	Runner RunnerID
	Tenant TenantID
	Secret string
}

// ---- Grants (permission model) ---------------------------------------------

// Operation is one entry in the small enumerable set a run may perform. No grant for an
// operation means denied (least-privilege by default).
type Operation string

const (
	OpPullAgent      Operation = "agent.pull"
	OpSpawn          Operation = "agent.spawn"
	OpRepoRead       Operation = "repo.read"
	OpRepoWrite      Operation = "repo.write"
	OpNetwork        Operation = "network"
	OpWriteBack      Operation = "provider.writeback"
	OpWorkspaceRead  Operation = "workspace.read"  // read across the shared root
	OpWorkspaceWrite Operation = "workspace.write" // write within the session workspace
	OpWorkspacePush  Operation = "workspace.push"  // publish the one allowed folder
)

// Grant is the scoped, integrity-protected authorization that travels with one dispatch.
// Scoped per run, not per identity; a runner cannot widen its own scope (Sig).
type Grant struct {
	Ops    []Operation
	Scope  map[string]string
	Expiry time.Time
	Sig    []byte
}

// Permits is a pure helper; integrity is GrantVerifier's job.
func (g Grant) Permits(op Operation) bool {
	for _, o := range g.Ops {
		if o == op {
			return true
		}
	}
	return false
}

// ---- Agents & catalog -------------------------------------------------------

type Form string

const (
	FormDefinition Form = "definition" // manifest: prompt + workflow + needs
	FormImage      Form = "image"      // packaged/container image reference
)

// AgentRef names an agent at a concrete or moving version, e.g. "code-fixer@1.2.0".
type AgentRef struct {
	Name    string
	Version string
}

type Version struct {
	Ref      AgentRef
	Form     Form
	Checksum string
	Payload  []byte
}

type Artifact struct {
	Ref     AgentRef
	Form    Form
	Content []byte
}

// ---- Bus: topics, messages, events -----------------------------------------

type Topic string

type Message struct {
	ID    string
	Topic Topic
	Kind  string
	Data  []byte
}

// Event is read off an SSE stream; ID is monotonic per stream for Last-Event-ID resume.
type Event struct {
	ID    string
	Topic Topic
	Kind  string
	Data  []byte
}

// Dispatch is the small message published to a runner's topic to start a run; references
// the exact version and carries the grant (artifact pulled lazily).
type Dispatch struct {
	Agent     AgentRef
	Grant     Grant
	Session   SessionID
	Runner    RunnerID
	Workspace *WorkspaceSpec // optional online-backed workspace to attach for the run
}

// Filter selects topics for a subscription; supports hierarchical wildcards ("session.s1.*")
// so a subscriber asks only for what it cares about (smart) and the broker need not
// materialize non-matching events for it (lazy).
type Filter struct {
	Topics []Topic
}

// ---- Sandbox execution ------------------------------------------------------

type DriverKind string

const (
	DriverLocal  DriverKind = "local"
	DriverDocker DriverKind = "docker"
)

type Limits struct {
	CPU      float64
	MemoryMB int
	Timeout  time.Duration // wall-clock; default 30m
}

// RunSpec is everything a driver needs to run one agent. Prompt is delivered over stdin,
// never argv.
type RunSpec struct {
	Agent   AgentRef
	Driver  DriverKind
	Workdir string
	Env     map[string]string
	Limits  Limits
	Grant   Grant
	Mounts  []Workspace // workspace (rw) + shared root (ro) mounted by the driver
}

type RunStatus string

const (
	StatusDone   RunStatus = "done"
	StatusFailed RunStatus = "failed"
)

type Result struct {
	Status   RunStatus
	ExitCode int
	Output   string
	Summary  string
}

type SandboxState string

const (
	SandboxRunning   SandboxState = "running"
	SandboxDestroyed SandboxState = "destroyed"
	SandboxCrashed   SandboxState = "crashed"
)

// ---- Today's poll/queue concepts (baseline scenarios) ----------------------

// Job is a unit of work in the current queue (baseline; reframed as bus backing store
// under c0002).
type Job struct {
	ID              string
	Status          string // queued|claimed|running|done|failed
	Workflow        string
	Instructions    string
	WritebackStatus string
}

// ============================================================================
// INTERFACES (capability contracts under review)
// ============================================================================

// Broker is the pluggable pub/sub backend (default Postgres LISTEN/NOTIFY; in-memory for
// tests; swappable) behind an unchanged SSE client contract.
type Broker interface {
	Publish(ctx context.Context, topic Topic, msg Message) error
	Subscribe(ctx context.Context, who Identity, filter Filter, fromEventID string) (Subscription, error)
	Ack(ctx context.Context, who Identity, msgID string) error
}

type Subscription interface {
	Events() <-chan Event
	Close() error
}

// Session is a live agent association; its control channel carries runner-bound control
// messages and PushToSandbox forwards a message down to the running agent.
type Session interface {
	ID() SessionID
	Control() Subscription
	PushToSandbox(ctx context.Context, msg Message) error
}

type Catalog interface {
	PublishVersion(ctx context.Context, by Identity, name, version string, form Form, payload []byte) (Version, error)
	Resolve(ctx context.Context, ref AgentRef) (Version, error)
	Pull(ctx context.Context, ref AgentRef, by Identity, g Grant) (Artifact, error)
	Dispatch(ctx context.Context, ref AgentRef, to RunnerID, g Grant) (SessionID, error)
}

type Registry interface {
	RegisterTenant(ctx context.Context, name string) (Tenant, error)
	IssueRegistrationToken(ctx context.Context, tenant TenantID) (string, error)
	EnrollRunner(ctx context.Context, regToken string) (RunnerIdentity, error)
	Presence(ctx context.Context, runner RunnerID) (bool, error)
	ListRunners(ctx context.Context, tenant TenantID) ([]RunnerID, error)
}

type Authenticator interface {
	AuthPAT(ctx context.Context, token string) (Identity, error)
	AuthRunner(ctx context.Context, credential string) (RunnerIdentity, error)
}

type GrantVerifier interface {
	Verify(ctx context.Context, g Grant) error
	Permits(g Grant, op Operation) bool
}

// SandboxDriver runs one agent through create→start→exec→stop→destroy; prompt on stdin.
type SandboxDriver interface {
	Kind() DriverKind
	Run(ctx context.Context, spec RunSpec) (Result, error)
}

// SandboxManager owns the per-run sandbox lifecycle, pulling, and crash handling.
type SandboxManager interface {
	Provision(ctx context.Context, d Dispatch) (SandboxID, error) // one agent per sandbox
	State(ctx context.Context, id SandboxID) (SandboxState, error)
	Destroy(ctx context.Context, id SandboxID) error // guaranteed teardown
	OnCrash(ctx context.Context, id SandboxID, h func(Result)) error
}

// AgentBackend is an installed AI CLI (claude code, codex, opencode).
type AgentBackend interface {
	Name() string
	Available() bool
	Run(ctx context.Context, spec RunSpec) (Result, error) // prompt via stdin
}

// Runner is the enrolled client loop: subscribe → pull → run → report → ack, with grant
// enforcement, reconnect/resume, and crash recovery.
type Runner interface {
	Enroll(ctx context.Context, hubURL, regToken string) error
	Start(ctx context.Context) error
}

// ============================================================================
// REAL-WORLD PLATFORM API (review surface — proposed, all skipped)
//
// The capabilities a production agent-hub needs beyond the core: scheduling, session
// management, interactive chat, live status/streaming, cancellation, and platform
// hardening (quotas, audit, notifications, artifacts, runner selection, secrets).
// ============================================================================

// ---- Scheduling -------------------------------------------------------------

type ScheduleID string

// ScheduleKind selects how a schedule fires.
type ScheduleKind string

const (
	ScheduleCron     ScheduleKind = "cron"     // recurring on a cron expression
	ScheduleInterval ScheduleKind = "interval" // recurring every Every
	ScheduleAt       ScheduleKind = "at"       // one-shot at At
)

// MissedPolicy decides what happens when a fire time elapses while no runner is online.
type MissedPolicy string

const (
	MissedSkip    MissedPolicy = "skip"     // drop the missed fire
	MissedCatchUp MissedPolicy = "catch_up" // run once on next availability
)

// ScheduleSpec describes a scheduled dispatch.
type ScheduleSpec struct {
	Kind   ScheduleKind
	Cron   string        // for ScheduleCron, e.g. "0 9 * * 1-5"
	Every  time.Duration // for ScheduleInterval
	At     time.Time     // for ScheduleAt
	TZ     string        // IANA timezone for cron, e.g. "Asia/Ho_Chi_Minh"
	Agent  AgentRef
	Target RunnerID
	Grant  Grant
	Missed MissedPolicy
}

type ScheduleState string

const (
	ScheduleActive   ScheduleState = "active"
	SchedulePaused   ScheduleState = "paused"
	ScheduleCanceled ScheduleState = "canceled"
)

// Scheduler dispatches agents on cron/interval/at-time schedules.
type Scheduler interface {
	Schedule(ctx context.Context, spec ScheduleSpec) (ScheduleID, error)
	Pause(ctx context.Context, id ScheduleID) error
	Resume(ctx context.Context, id ScheduleID) error
	Cancel(ctx context.Context, id ScheduleID) error
	List(ctx context.Context, tenant TenantID) ([]ScheduleID, error)
}

// ---- Sessions & interactive chat -------------------------------------------

type SessionStatus string

const (
	SessionActive SessionStatus = "active"
	SessionIdle   SessionStatus = "idle"
	SessionClosed SessionStatus = "closed"
)

// SessionInfo is the management view of a live agent association.
type SessionInfo struct {
	ID      SessionID
	Tenant  TenantID
	Runner  RunnerID
	Agent   AgentRef
	Status  SessionStatus
	Owner   AccountID
	Created time.Time
}

// SessionManager owns the session lifecycle (separate from the bus Session primitive,
// which is the live wire endpoint Attach returns).
type SessionManager interface {
	Create(ctx context.Context, d Dispatch) (SessionInfo, error)
	Get(ctx context.Context, id SessionID) (SessionInfo, error)
	List(ctx context.Context, tenant TenantID) ([]SessionInfo, error)
	Attach(ctx context.Context, id SessionID) (Session, error) // live endpoint for chat/stream
	Close(ctx context.Context, id SessionID) error
}

// Role labels a chat turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

type ChatMessage struct {
	Role    Role
	Content string
}

// ChatEvent is a streamed unit of an assistant turn.
type ChatEvent struct {
	Kind    string // "token" | "message" | "done" | "error"
	Content string
}

// Conversation is an interactive, multi-turn chat with a running agent in a session.
type Conversation interface {
	Send(ctx context.Context, m ChatMessage) error
	Stream() <-chan ChatEvent // assistant tokens/events for the current turn
	History(ctx context.Context) ([]ChatMessage, error)
	Cancel(ctx context.Context) error // interrupt the in-flight turn
}

// ---- Live status & streaming (agent → hub comms) ---------------------------

// RunEvent is an upstream event the runner/agent emits about a run.
type RunEvent struct {
	Kind string // "progress" | "log" | "output" | "status"
	Data []byte
}

// RunEvents carries upstream run telemetry and run-level control.
type RunEvents interface {
	Emit(ctx context.Context, runID string, ev RunEvent) error         // runner → hub
	Subscribe(ctx context.Context, runID string) (Subscription, error) // client tails a run
	Status(ctx context.Context, runID string) (RunStatus, error)
	Cancel(ctx context.Context, runID string, force bool) error // graceful then forced
}

// ---- Platform hardening -----------------------------------------------------

type Limit struct {
	MaxConcurrentRuns int
	MaxDispatchPerMin int
}

// Quota enforces per-tenant limits.
type Quota interface {
	Allow(ctx context.Context, tenant TenantID, op Operation) (bool, error)
	Limits(ctx context.Context, tenant TenantID) (Limit, error)
}

type AuditEntry struct {
	Actor  Identity
	Action string
	Target string
	At     time.Time
}

// AuditLog records and queries security-relevant actions.
type AuditLog interface {
	Record(ctx context.Context, e AuditEntry) error
	Query(ctx context.Context, tenant TenantID) ([]AuditEntry, error)
}

type NotifyEvent struct {
	Kind   string // "run.done" | "run.failed" | ...
	RunID  string
	Target string // outbound webhook URL or channel
}

// Notifier delivers outbound notifications (webhooks/chat) on platform events.
type Notifier interface {
	Notify(ctx context.Context, ev NotifyEvent) error
}

type ArtifactRef struct {
	RunID    string
	Name     string
	Checksum string
}

// ArtifactStore persists and serves run outputs/artifacts.
type ArtifactStore interface {
	Put(ctx context.Context, ref ArtifactRef, content []byte) error
	Get(ctx context.Context, ref ArtifactRef) ([]byte, error)
	List(ctx context.Context, runID string) ([]ArtifactRef, error)
}

// RunnerSelector picks a target runner for a dispatch (load-balancing / affinity).
type RunnerSelector interface {
	Select(ctx context.Context, d Dispatch) (RunnerID, error)
}

// SecretInjector injects grant-scoped secrets into a sandbox (never via argv/logs).
type SecretInjector interface {
	Inject(ctx context.Context, sandbox SandboxID, secrets map[string]string) error
}

// ============================================================================
// STORAGE & WORKSPACES (review surface — proposed, all skipped)
//
// Each session/sandbox can attach an online-backed workspace it writes into; files sync
// to S3-compatible storage. A shared root is readable across all published folders, while
// writes/pushes are confined to the grant's allowed prefix. Three clean layers:
//   ObjectStore (S3-compatible backend) → WorkspaceManager (mount/sync/scope/hooks) →
//   WorkspaceFS (the agent-facing file view inside the sandbox).
// ============================================================================

// ---- S3-compatible object store --------------------------------------------

type ObjectRef struct {
	Bucket string
	Key    string
}

type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time
}

type PutOpts struct {
	ContentType string
	Checksum    string // e.g. sha256; verified on read
}

// ObjectStore is the S3-compatible backend (AWS S3 / MinIO / Cloudflare R2 via
// endpoint+region+bucket+credentials). It is the only component that holds store creds;
// agents receive presigned URLs, never the credentials.
type ObjectStore interface {
	PutObject(ctx context.Context, ref ObjectRef, content []byte, opts PutOpts) error
	GetObject(ctx context.Context, ref ObjectRef) ([]byte, error)
	HeadObject(ctx context.Context, ref ObjectRef) (ObjectInfo, error)
	ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error)
	DeleteObject(ctx context.Context, ref ObjectRef) error
	PresignGetURL(ctx context.Context, ref ObjectRef, ttl time.Duration) (string, error)
	PresignPutURL(ctx context.Context, ref ObjectRef, ttl time.Duration) (string, error)
	// PutMultipart streams a large object in parts (returns the final ETag).
	PutMultipart(ctx context.Context, ref ObjectRef, parts [][]byte) (string, error)
}

// ---- Workspace base code & lifecycle hooks ---------------------------------

// BaseKind selects how a workspace's base code is materialized before any hook runs.
type BaseKind string

const (
	BaseNone    BaseKind = "none"
	BaseGit     BaseKind = "git"     // clone Ref @ Rev
	BaseArchive BaseKind = "archive" // unpack an archive (Ref = object key / url)
	BaseStore   BaseKind = "store"   // copy a template prefix from the ObjectStore
)

// BaseSpec seeds a workspace (e.g. clone a git repo) before the agent runs.
type BaseSpec struct {
	Kind BaseKind
	Ref  string // git URL / archive key / store prefix
	Rev  string // branch / tag / sha for git
}

// HookStage is a point in the sandbox/workspace lifecycle where a hook script runs.
type HookStage string

const (
	HookInit     HookStage = "init"      // after base materialized: clone done, install deps
	HookPreRun   HookStage = "pre_run"   // before the agent
	HookPostRun  HookStage = "post_run"  // after the agent
	HookPreSync  HookStage = "pre_sync"  // before pushing to remote
	HookPostSync HookStage = "post_sync" // after pushing to remote
)

// Hook is a lifecycle script the sandbox runs. Script is fed over stdin, never argv.
type Hook struct {
	Name   string
	Stage  HookStage
	Script string
}

// ---- Workspace mounting, sync & scope --------------------------------------

type WorkspaceID string

type WorkspaceMode string

const (
	WorkspaceRW WorkspaceMode = "rw"
	WorkspaceRO WorkspaceMode = "ro"
)

type SyncMode string

const (
	SyncContinuous SyncMode = "continuous" // mirror writes as they happen
	SyncOnFlush    SyncMode = "on_flush"   // sync on explicit flush / detach
	SyncManual     SyncMode = "manual"     // only on Sync()
)

type SyncResult struct {
	Pushed int
	Pulled int
	Failed int
	At     time.Time
}

// WorkspaceSpec describes a workspace to attach to a session.
type WorkspaceSpec struct {
	MountPath    string // where it mounts inside the sandbox, e.g. "/workspace"
	RemotePrefix string // object-store prefix, e.g. "<tenant>/<session>/"
	Mode         WorkspaceMode
	Sync         SyncMode
	SharedRoots  []string // read-only shared prefixes the agent may read across
	Base         BaseSpec // base code to seed before run
	Hooks        []Hook   // lifecycle hooks the sandbox runs
}

// Workspace is an attached, mounted workspace bound to a remote prefix.
type Workspace struct {
	ID        WorkspaceID
	Session   SessionID
	MountPath string
	Bucket    string
	Prefix    string
	Mode      WorkspaceMode
}

// WorkspaceManager attaches workspaces to sessions, wires sync to the ObjectStore,
// manages the shared root, runs base-code bootstrap + lifecycle hooks, and publishes the
// one allowed folder.
type WorkspaceManager interface {
	Attach(ctx context.Context, session SessionID, spec WorkspaceSpec) (Workspace, error)
	Get(ctx context.Context, id WorkspaceID) (Workspace, error)
	Detach(ctx context.Context, id WorkspaceID) error // final flush + unmount
	Sync(ctx context.Context, id WorkspaceID) (SyncResult, error)
	Status(ctx context.Context, id WorkspaceID) (SyncResult, error)
	MountSharedRoot(ctx context.Context, session SessionID) (Workspace, error) // read-only union
	// Publish promotes one allowed sub-path into the shared/artifacts namespace.
	Publish(ctx context.Context, id WorkspaceID, srcPath, dest string) error
	// Bootstrap materializes Base then runs init hooks.
	Bootstrap(ctx context.Context, id WorkspaceID) (Result, error)
	// RunHooks fires the hooks for a lifecycle stage (the runner drives pre/post stages).
	RunHooks(ctx context.Context, id WorkspaceID, stage HookStage) (Result, error)
}

// WorkspaceFS is the agent-facing file view inside the sandbox. Reads may span the shared
// root; writes are confined to the grant's writable prefix (path traversal blocked).
type WorkspaceFS interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error // OpWorkspaceWrite scope
	List(ctx context.Context, path string) ([]string, error)
	Remove(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (ObjectInfo, error)
}

// ============================================================================
// World — the harness each scenario operates on (the complete system setup from
// harness.md). Methods are unimplemented design stubs (panic): scenarios are skipped, so
// they never run; they exist so the behavior reads and type-checks.
// ============================================================================

type World struct {
	// target capability handles (nil in design; real once built)
	Bus        Broker
	Catalog    Catalog
	Registry   Registry
	Auth       Authenticator
	Grants     GrantVerifier
	SandboxMgr SandboxManager
	Runner     Runner

	// real-world platform handles
	Scheduler Scheduler
	Sessions  SessionManager
	Runs      RunEvents
	Quota     Quota
	Audit     AuditLog
	Notifier  Notifier
	Artifacts ArtifactStore
	Selector  RunnerSelector
	Secrets   SecretInjector

	// storage & workspaces
	Store      ObjectStore
	Workspaces WorkspaceManager

	// fixtures
	Tenant   Tenant
	RunnerID RunnerIdentity
	Agent    AgentRef
	Grant    Grant
	Session  Session

	// scratch space shared across steps
	state map[string]any
}

func ctx() context.Context { return context.Background() }

func (w *World) set(k string, v any) { /* design stub */ panic("design-only World.set") }
func (w *World) get(k string) any    { panic("design-only World.get") }

// --- today (baseline) harness verbs ---
func (w *World) StartHub() error                                         { panic("design-only") }
func (w *World) ConfigBlank(secret string)                               { panic("design-only") }
func (w *World) Healthz() (int, string)                                  { panic("design-only") }
func (w *World) SeedAccount(pat string) AccountID                        { panic("design-only") }
func (w *World) ConnectProvider(token, secret string)                    { panic("design-only") }
func (w *World) CreateProfile(name, body, backend, harness string)       { panic("design-only") }
func (w *World) RegisterRuntime(code, label string) (RunnerID, string)   { panic("design-only") }
func (w *World) WatchContainer(board string)                             { panic("design-only") }
func (w *World) PostWebhook(comment, deliveryID string, signed bool) int { panic("design-only") }
func (w *World) Claim(token string) *Job                                 { panic("design-only") }
func (w *World) Ack(token, jobID, status, summary string) error          { panic("design-only") }
func (w *World) Heartbeat(token, jobID string) error                     { panic("design-only") }
func (w *World) JobStatus(jobID string) string                           { panic("design-only") }
func (w *World) Writebacks() int                                         { panic("design-only") }
func (w *World) LastComment() string                                     { panic("design-only") }
func (w *World) RunCLI(args ...string) (string, error)                   { panic("design-only") }
func (w *World) ConfigFileMode() uint32                                  { panic("design-only") }
func (w *World) DaemonStatus() string                                    { panic("design-only") }
func (w *World) DetectBackend() AgentBackend                             { panic("design-only") }

// --- target harness verbs ---
func (w *World) RegisterTenant(name string) Tenant                              { panic("design-only") }
func (w *World) IssueRegToken(tenant TenantID) string                           { panic("design-only") }
func (w *World) EnrollRunner(regToken string) (RunnerIdentity, error)           { panic("design-only") }
func (w *World) OpenSession(id SessionID, filter Filter) Session                { panic("design-only") }
func (w *World) PublishVersion(name, version string, form Form) Version         { panic("design-only") }
func (w *World) Dispatch(ref AgentRef, to RunnerID, g Grant) (SessionID, error) { panic("design-only") }
func (w *World) Subscribe(filter Filter, fromID string) Subscription            { panic("design-only") }
func (w *World) Publish(topic Topic, kind string) error                         { panic("design-only") }
func (w *World) FakeAgent(name string, mode string)                             { panic("design-only") }
func (w *World) Driver(kind DriverKind) SandboxDriver                           { panic("design-only") }
func (w *World) Backend(name string) AgentBackend                               { panic("design-only") }
func (w *World) StartRunner() error                                             { panic("design-only") }

// --- real-world platform harness verbs ---
func (w *World) Chat(id SessionID) Conversation { panic("design-only") }
func (w *World) RunID() string                  { panic("design-only") }
func (w *World) Advance(d time.Duration)        { panic("design-only") } // fast-forward virtual clock for schedules

// storage & workspace verbs
func (w *World) AttachWorkspace(session SessionID, spec WorkspaceSpec) Workspace {
	panic("design-only")
}
func (w *World) FS(id WorkspaceID) WorkspaceFS            { panic("design-only") }
func (w *World) RemoteObjects(prefix string) []ObjectInfo { panic("design-only") }

func (w *World) expect(cond bool, format string, args ...any) { panic("design-only") }

// msg builds a Message for a topic/kind (helper used by bus scenarios).
func msg(topic Topic, kind string) Message { return Message{Topic: topic, Kind: kind} }

// grant builds a least-privilege grant over the given operations.
func grant(ops ...Operation) Grant { return Grant{Ops: ops} }

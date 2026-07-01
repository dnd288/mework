package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"mework/libs/client/catalog"
	"mework/libs/client/osproc"
	"mework/libs/client/runner"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/config"
	"mework/libs/shared/grant"
)

var daemonCmd = &cobra.Command{
	Use:     "daemon",
	Short:   "Manage the local agent-runtime daemon",
	GroupID: groupRuntime,
}

var daemonForeground bool
var daemonOffline bool
var workspaceDir string

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent daemon (background by default; --foreground to run in-process)",
	RunE: func(cmd *cobra.Command, args []string) error {
		prof := profile()
		if daemonOffline {
			return runOfflineForeground(prof)
		}
		if running, pid := runner.IsRunning(prof); running {
			fmt.Printf("daemon already running (pid %d)\n", pid)
			return nil
		}
		if daemonForeground {
			return runForeground(prof)
		}
		return spawnBackground(prof)
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		prof := profile()
		running, pid := runner.IsRunning(prof)
		if !running {
			fmt.Println("daemon is not running")
			_ = runner.RemovePID(prof)
			return nil
		}
		// Prefer graceful shutdown via the health port.
		if runner.RequestShutdown(prof, 3*time.Second) {
			fmt.Println("daemon shutting down")
			return nil
		}
		// Fall back to signaling the process directly.
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
		_ = runner.RemovePID(prof)
		fmt.Println("daemon stopped")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		prof := profile()
		if running, pid := runner.IsRunning(prof); running {
			fmt.Printf("running (pid %d, health port %d)\n", pid, runner.HealthPort(prof))
		} else {
			fmt.Println("stopped")
		}
		return nil
	},
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		prof := profile()
		if running, _ := runner.IsRunning(prof); running {
			runner.RequestShutdown(prof, 3*time.Second)
			time.Sleep(500 * time.Millisecond)
		}
		return spawnBackground(prof)
	},
}

var daemonLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print the daemon log (use -f to follow)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tailLog(profile(), daemonLogsFollow)
	},
}

var daemonLogsFollow bool

func init() {
	daemonStartCmd.Flags().BoolVar(&daemonForeground, "foreground", false, "run the daemon in the foreground")
	daemonStartCmd.Flags().BoolVar(&daemonOffline, "offline", false, "run the daemon in offline mode (no hub, no provider)")
	daemonStartCmd.Flags().StringVar(&workspaceDir, "workspace", "", "workspace directory for offline mode")
	daemonLogsCmd.Flags().BoolVarP(&daemonLogsFollow, "follow", "f", false, "follow the log output")
	daemonCmd.AddCommand(daemonStartCmd, daemonStopCmd, daemonStatusCmd, daemonRestartCmd, daemonLogsCmd)
}

// spawnBackground re-execs this binary with --foreground, detached.
func spawnBackground(prof string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := runner.OpenLogFile(prof)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmdArgs := []string{"daemon", "start", "--foreground"}
	if prof != "" {
		cmdArgs = append(cmdArgs, "--profile", prof)
	}
	child := exec.Command(exe, cmdArgs...)
	osproc.ConfigureDetached(child, logFile)
	if err := child.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	// Capture pid before Release (which zeroes it on some platforms), then
	// release so the child keeps running after we exit.
	pid := child.Process.Pid
	_ = child.Process.Release()
	fmt.Printf("daemon started (pid %d)\n", pid)
	return nil
}

// runForeground runs the daemon loop in-process until interrupted or /shutdown.
func runForeground(prof string) error {
	if err := runner.WritePID(prof); err != nil {
		return err
	}
	defer runner.RemovePID(prof)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	health, err := runner.StartHealthServer(prof, cancel)
	if err != nil {
		return err
	}
	defer health.Close()

	cfg, err := config.LoadConfig(prof)
	if err != nil {
		return err
	}

	// Check for enrolled identity.
	runnerID, secret, err := config.LoadIdentity()
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}
	if runnerID == "" {
		// Attempt auto-migration from old rt_token.
		if migErr := runner.AutoMigrate(); migErr != nil {
			return fmt.Errorf("not enrolled — run `mework runner enroll --url <hub> --token <reg>` first")
		}
		runnerID, secret, err = config.LoadIdentity()
		if err != nil || runnerID == "" {
			return fmt.Errorf("not enrolled — run `mework runner enroll --url <hub> --token <reg>` first")
		}
	}

	// Wire the catalog-backed definition resolver for interactive (open-session)
	// dispatches. The local-claude@1.0.0 definition (engine local, backend claude)
	// resolves over the server catalog; FileDefinitionResolver is the local
	// fallback. The runner package cannot import catalog directly (import cycle),
	// so the factory is injected here.
	runner.SetSessionResolverFactory(func(catalogURL string) runner.DefinitionResolver {
		return &catalog.HTTPDefinitionResolver{BaseURL: catalogURL}
	})
	// Workspace-bound dispatches (mework sandbox start -w .) resolve the
	// definition from the workspace's mework.yml and bind the sandbox to the dir.
	runner.SetSessionWorkspaceResolverFactory(func(workspaceDir string) runner.DefinitionResolver {
		return &catalog.FileDefinitionResolver{WorkspaceDir: workspaceDir}
	})

	engine := runner.NewEngine(runnerID, secret, cfg.ServerURL, cfg.ServerURL)
	if err := engine.Start(ctx); err != nil {
		return err
	}
	// Block until a signal (SIGINT/SIGTERM) or health /shutdown arrives.
	// Engine.Start spawns goroutines and returns; without the block the
	// process would exit immediately and kill them.
	<-ctx.Done()
	return nil
}

// runOfflineForeground validates the offline-mode setup and starts the
// workspace-bound session without hub enrollment or network dependencies.
// It returns nil when all validations pass and the session is ready.
func runOfflineForeground(prof string) error {
	if workspaceDir == "" {
		return fmt.Errorf("--workspace is required in offline mode")
	}

	info, err := os.Stat(workspaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no such directory: %s", workspaceDir)
		}
		return fmt.Errorf("workspace directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", workspaceDir)
	}

	// Load and validate mework.yml.
	meta, err := catalog.LoadWorkspaceConfig(workspaceDir)
	if err != nil {
		if errors.Is(err, runner.ErrDefinitionNotFound) {
			return fmt.Errorf("no mework.yml found in workspace: %s", workspaceDir)
		}
		return fmt.Errorf("workspace config: %w", err)
	}

	if err := runner.ValidateOfflineEngine(meta); err != nil {
		return err
	}

	// Wire a self-contained in-process session: in-memory broker, local-only
	// grant, and a file-system definition resolver from the workspace.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	_ = mgr // session manager; kept for future lifecycle use

	key := []byte("offline-key")
	localGrant, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, key)
	if err != nil {
		return fmt.Errorf("mint grant: %w", err)
	}
	caller := runner.Caller{
		Account: "offline",
		Tenant:  "offline",
		Grant:   localGrant,
	}

	sess, err := runner.StartWorkspaceSession(ctx, runner.StartOptions{
		Ref:          meta.Name + "@" + meta.Version,
		Resolver:     &catalog.FileDefinitionResolver{WorkspaceDir: workspaceDir},
		WorkspaceDir: workspaceDir,
		Caller:       caller,
		GrantKey:     key,
		Broker:       broker,
		Sessions:     mgr,
		// ManagerFor is nil — falls through to runtime.NewManagerFor ("local" engine).
	})
	if err != nil {
		return fmt.Errorf("start workspace session: %w", err)
	}

	// Start the Unix-socket IPC listener so mework send can connect.
	srv, err := runner.NewOfflineServer(workspaceDir, sess)
	if err != nil {
		_ = sess.Close(ctx, caller)
		return fmt.Errorf("offline server: %w", err)
	}

	// Attach message policy from mework.yml (if defined).
	if meta.Policy != nil {
		srv.SetPolicy(meta.Policy)
	}

	// Register as a local offline agent so "mework agent list" and
	// "mework agent send <name>" can find it without a --workspace flag.
	sockPath, _ := runner.SocketPath(workspaceDir)
	agentInfo := runner.OfflineAgentInfo{
		Name:       meta.Name,
		SocketPath: sockPath,
		Status:     "online",
		Workspace:  workspaceDir,
		Backend:    meta.Backend,
	}
	if regErr := runner.RegisterOfflineAgent(agentInfo); regErr != nil {
		_ = sess.Close(ctx, caller)
		return fmt.Errorf("register agent: %w", regErr)
	}
	defer runner.UnregisterOfflineAgent(meta.Name)

	fmt.Printf("offline agent %q ready\n", meta.Name)

	// Block until SIGINT/SIGTERM, then clean up.
	if err := srv.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_ = sess.Close(ctx, caller)
		return err
	}
	return sess.Close(ctx, caller)
}

// tailLog prints the log file, optionally following appended lines.
func tailLog(prof string, follow bool) error {
	f, err := os.Open(config.LogPath(prof))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("(no log yet)")
			return nil
		}
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	printAll(reader)
	if !follow {
		return nil
	}
	for {
		time.Sleep(500 * time.Millisecond)
		printAll(reader)
	}
}

func printAll(r *bufio.Reader) {
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			fmt.Print(line)
		}
		if err != nil {
			break
		}
	}
}

package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"mework/libs/client/osproc"
	"mework/libs/client/runner"
	"mework/libs/shared/config"
)

var daemonCmd = &cobra.Command{
	Use:     "daemon",
	Short:   "Manage the local agent-runtime daemon",
	GroupID: groupRuntime,
}

var daemonForeground bool

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent daemon (background by default; --foreground to run in-process)",
	RunE: func(cmd *cobra.Command, args []string) error {
		prof := profile()
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

	engine := runner.NewEngine(runnerID, secret, cfg.ServerURL, cfg.ServerURL)
	return engine.Start(ctx)
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

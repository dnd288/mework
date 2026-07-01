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

// mezonWorkerProfile is a fixed profile name used for the mezon-worker's
// PID and log files, isolating its lifecycle from the main daemon.
const mezonWorkerProfile = "mezon-worker"

var mezonWorkerCmd = &cobra.Command{
	Use:   "mezon-worker",
	Short: "Manage the standalone Mezon bot worker process",
}

var mezonWorkerForeground bool

var mezonWorkerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Mezon worker (background by default; --foreground for in-process)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if running, pid := runner.IsRunning(mezonWorkerProfile); running {
			fmt.Printf("mezon-worker already running (pid %d)\n", pid)
			return nil
		}

		creds := resolveMezonCreds()
		if creds == nil {
			return fmt.Errorf("mezon credentials not configured — run 'mework provider mezon set' first")
		}

		if mezonWorkerForeground {
			return runMezonWorkerForeground(creds)
		}
		return spawnMezonWorkerBackground(creds)
	},
}

var mezonWorkerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running Mezon worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, pid := runner.IsRunning(mezonWorkerProfile)
		if !running {
			fmt.Println("mezon-worker is not running")
			_ = runner.RemovePID(mezonWorkerProfile)
			return nil
		}

		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(syscall.SIGTERM)
			done := make(chan struct{})
			go func() {
				for i := 0; i < 30; i++ {
					if ok, _ := runner.IsRunning(mezonWorkerProfile); !ok {
						close(done)
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
				_ = proc.Signal(syscall.SIGKILL)
				close(done)
			}()
			<-done
		}

		_ = runner.RemovePID(mezonWorkerProfile)
		fmt.Println("mezon-worker stopped")
		return nil
	},
}

var mezonWorkerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Mezon worker status",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, pid := runner.IsRunning(mezonWorkerProfile)
		if running {
			fmt.Printf("running (pid %d)\n", pid)
		} else {
			fmt.Println("stopped")
		}

		creds := resolveMezonCreds()
		if creds != nil {
			fmt.Printf("configured: app %s\n", creds.AppID)
		} else {
			fmt.Println("not configured — run 'mework provider mezon set'")
		}
		return nil
	},
}

var mezonWorkerLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print the Mezon worker log (use -f to follow)",
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		return tailMezonLog(follow)
	},
}

func init() {
	mezonWorkerStartCmd.Flags().BoolVar(&mezonWorkerForeground, "foreground", false, "run the worker in the foreground")
	mezonWorkerLogsCmd.Flags().BoolP("follow", "f", false, "follow the log output")
	mezonWorkerCmd.AddCommand(mezonWorkerStartCmd, mezonWorkerStopCmd, mezonWorkerStatusCmd, mezonWorkerLogsCmd)
}

// resolveMezonCreds returns Mezon credentials from config or env vars.
func resolveMezonCreds() *config.MezonCredentials {
	// Env vars take precedence.
	if v := os.Getenv("MEZON_APP_ID"); v != "" {
		return &config.MezonCredentials{
			AppID:   v,
			APIKey:  os.Getenv("MEZON_API_KEY"),
			BaseURL: os.Getenv("MEZON_BASE_URL"),
		}
	}

	// Fall back to stored config.
	cfg, err := config.LoadConfig(profile())
	if err != nil {
		return nil
	}
	return cfg.Mezon
}

// runMezonWorkerForeground runs the worker process in the foreground.
func runMezonWorkerForeground(creds *config.MezonCredentials) error {
	if err := runner.WritePID(mezonWorkerProfile); err != nil {
		return err
	}
	defer runner.RemovePID(mezonWorkerProfile)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	workerExe := findMezonWorkerBinary()
	if workerExe == "" {
		return fmt.Errorf("mework-mezon-worker binary not found")
	}

	workerEnv := workerCommandEnv(creds)
	workerCmd := exec.CommandContext(ctx, workerExe)
	workerCmd.Env = workerEnv
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr

	fmt.Printf("mezon-worker starting (pid %d)\n", os.Getpid())
	if err := workerCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	return nil
}

// spawnMezonWorkerBackground spawns the worker as a detached background process.
func spawnMezonWorkerBackground(creds *config.MezonCredentials) error {
	workerExe := findMezonWorkerBinary()
	if workerExe == "" {
		return fmt.Errorf("mework-mezon-worker binary not found")
	}

	logFile, err := runner.OpenLogFile(mezonWorkerProfile)
	if err != nil {
		return err
	}
	defer logFile.Close()

	workerEnv := workerCommandEnv(creds)
	child := exec.Command(workerExe)
	child.Env = workerEnv
	osproc.ConfigureDetached(child, logFile)

	if err := child.Start(); err != nil {
		return fmt.Errorf("spawn mezon-worker: %w", err)
	}

	pid := child.Process.Pid
	_ = child.Process.Release()

	if err := runner.WritePID(mezonWorkerProfile); err != nil {
		fmt.Printf("warning: could not write pid file: %v\n", err)
	}

	fmt.Printf("mezon-worker started (pid %d)\n", pid)
	return nil
}

// workerCommandEnv builds the environment variables for the worker process.
// It starts with the current process env, injects Mezon credentials, and adds
// server connection details from config or existing env.
func workerCommandEnv(creds *config.MezonCredentials) []string {
	env := os.Environ()

	// Inject Mezon credentials.
	env = append(env, "MEZON_APP_ID="+creds.AppID)
	env = append(env, "MEZON_API_KEY="+creds.APIKey)
	if creds.BaseURL != "" {
		env = append(env, "MEZON_BASE_URL="+creds.BaseURL)
	}

	// MEWORK_TOKEN: check env first, then config.
	if os.Getenv("MEWORK_TOKEN") == "" {
		if cfg, err := config.LoadConfig(profile()); err == nil && cfg.RuntimeToken != "" {
			env = append(env, "MEWORK_TOKEN="+cfg.RuntimeToken)
		}
	}

	// MEWORK_SERVER_URL: check env, then config, then default.
	if os.Getenv("MEWORK_SERVER_URL") == "" {
		serverURL := "http://localhost:8080"
		if cfg, err := config.LoadConfig(profile()); err == nil {
			if cfg.ServerURL != "" {
				serverURL = cfg.ServerURL
			} else if cfg.BaseURL != "" {
				serverURL = cfg.BaseURL
			}
		}
		env = append(env, "MEWORK_SERVER_URL="+serverURL)
	}

	return env
}

// findMezonWorkerBinary looks for the mework-mezon-worker binary adjacent to
// the current executable, or in the same directory's parent bin/.
func findMezonWorkerBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := exe
	if idx := lastPathSep(dir); idx >= 0 {
		dir = dir[:idx+1]
	}
	candidates := []string{
		dir + "mework-mezon-worker",
		dir + "../bin/mework-mezon-worker",
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		candidates = append(candidates, gopath+"/bin/mework-mezon-worker")
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

// lastPathSep returns the index of the last path separator, or -1.
func lastPathSep(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// tailMezonLog prints the worker log file, optionally following appended lines.
func tailMezonLog(follow bool) error {
	f, err := os.Open(config.LogPath(mezonWorkerProfile))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("(no log yet)")
			return nil
		}
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			fmt.Print(line)
		}
		if err != nil {
			if follow {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			break
		}
	}
	return nil
}

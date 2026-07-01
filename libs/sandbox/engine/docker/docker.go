// Package docker implements the ports.SandboxDriver interface for Docker
// containers. Each agent runs in its own container for process isolation.
// The driver uses the docker CLI (via exec) rather than the Docker SDK so
// that local-only builds add no third-party dependency.
package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// Driver implements ports.SandboxDriver for Docker containers.
type Driver struct{}

// New creates a new Docker Driver.
func New() *Driver { return &Driver{} }

// Caps returns the capabilities of this driver.
func (d *Driver) Caps() core.SandboxCaps {
	return core.SandboxCaps{
		IsIsolated:  true,
		IsRemote:    false,
		SupportsGPU: false,
		SupportsNet: false,
		MaxMemoryMB: 2048,
		MaxDiskMB:   10240,
		DriverName:  "docker",
	}
}

// dockerSandbox is a running Docker container sandbox.
type dockerSandbox struct {
	id          string
	containerID string
	workDir     string
}

func (s *dockerSandbox) ID() string { return s.id }

// Exec runs a command inside the Docker container.
func (s *dockerSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	args := append([]string{"exec", "-i", s.containerID}, command...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return -1, err
	}
	return 0, nil
}

// Mount copies a source path into the container via docker cp.
func (s *dockerSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	source := workspace.Path
	cpCmd := exec.CommandContext(ctx, "docker", "cp", source, s.containerID+":"+targetPath)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp: %w\n%s", err, string(out))
	}
	return nil
}

// Signals sends a kill signal to the container.
func (s *dockerSandbox) Signals(ctx context.Context, sig string) error {
	return exec.CommandContext(ctx, "docker", "kill", "-s", sig, s.containerID).Run()
}

// resolveImage returns the image to use.
func resolveImage(spec core.RunSpec) string {
	if spec.Image != "" {
		return spec.Image
	}
	return "ubuntu:22.04"
}

// Start creates and starts a Docker container for the agent run.
func (d *Driver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	image := resolveImage(spec)

	// Pull the image if not present locally.
	if err := ensureImage(ctx, image); err != nil {
		return nil, fmt.Errorf("ensure image %s: %w", image, err)
	}

	workDir := spec.SandboxID
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "mework-sandbox-docker", spec.AgentID)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	containerID, err := d.createContainer(ctx, spec, image, workDir)
	if err != nil {
		return nil, err
	}

	// Start the container.
	startCmd := exec.CommandContext(ctx, "docker", "start", containerID)
	if out, err := startCmd.CombinedOutput(); err != nil {
		_ = exec.CommandContext(context.Background(), "docker", "rm", "-f", containerID).Run()
		return nil, fmt.Errorf("docker start: %w\n%s", err, string(out))
	}

	return &dockerSandbox{
		id:          spec.SandboxID,
		containerID: containerID,
		workDir:     workDir,
	}, nil
}

func (d *Driver) createContainer(ctx context.Context, spec core.RunSpec, image, workDir string) (string, error) {
	args := []string{
		"create",
		"--rm",
		"--workdir", "/work",
		"--mount", fmt.Sprintf("type=bind,source=%s,target=/work", workDir),
	}

	if rl := spec.ResourceLimits; rl != nil {
		if rl.Memory != "" {
			args = append(args, "--memory", rl.Memory)
		}
		if rl.CPU != "" {
			args = append(args, "--cpus", rl.CPU)
		}
	}

	for k, v := range spec.Env {
		args = append(args, "-e", k+"="+v)
	}

	// Use sandbox ID for the container name if set.
	if spec.SandboxID != "" {
		args = append(args, "--name", containerName(spec.SandboxID))
	}

	args = append(args, image, "sleep", "infinity")

	var createOut bytes.Buffer
	createCmd := exec.CommandContext(ctx, "docker", args...)
	createCmd.Stdout = &createOut
	createCmd.Stderr = &createOut
	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("docker create: %w\n%s", err, createOut.String())
	}
	return strings.TrimSpace(createOut.String()), nil
}

// Stop stops the container gracefully.
func (d *Driver) Stop(ctx context.Context, sandboxID string) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", containerName(sandboxID))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %w\n%s", err, string(out))
	}
	return nil
}

// Destroy removes the container forcibly.
func (d *Driver) Destroy(ctx context.Context, sandboxID string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName(sandboxID))
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "No such container") {
			return nil
		}
		return fmt.Errorf("docker rm: %w\n%s", err, string(out))
	}
	return nil
}

// ensureImage pulls the image if not present locally.
func ensureImage(ctx context.Context, image string) error {
	checkCmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	if err := checkCmd.Run(); err == nil {
		return nil
	}
	pullCmd := exec.CommandContext(ctx, "docker", "pull", image)
	if out, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull: %w\n%s", err, string(out))
	}
	return nil
}

func containerName(id string) string {
	return "mework-" + id
}

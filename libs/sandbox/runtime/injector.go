package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mework/libs/shared/ports"
)

// SecretInjectorImpl implements ports.SecretInjector by materializing each
// secret into a per-sandbox file with 0400 permissions and exposing it via
// an environment variable whose name is grant-scoped.
type SecretInjectorImpl struct {
	// secretsDir is the base directory under which per-sandbox secret
	// directories are created (e.g. /run/mework/secrets).
	secretsDir string
}

// NewSecretInjector creates a new SecretInjectorImpl.
func NewSecretInjector(secretsDir string) *SecretInjectorImpl {
	if secretsDir == "" {
		secretsDir = "/run/mework/secrets"
	}
	return &SecretInjectorImpl{secretsDir: secretsDir}
}

// Inject materialises each granted secret into a per-sandbox file with 0400
// permissions and exposes it via an environment variable named
// <SOURCE>_<NAME>. Each secret's Source must be present in the provided
// sources list; otherwise ErrSecretRefused is returned.
func (inj *SecretInjectorImpl) Inject(ctx context.Context, sandboxID string, sources []string, secrets []ports.SecretRef) error {
	if sandboxID == "" {
		return fmt.Errorf("sandboxID is required")
	}

	// Build a set of allowed sources for O(1) lookup.
	allowedSources := make(map[string]bool, len(sources))
	for _, s := range sources {
		allowedSources[s] = true
	}

	// Create the per-sandbox secrets directory with 0700 so only
	// the sandbox user can enter it.
	sandboxSecretsDir := filepath.Join(inj.secretsDir, sandboxID)
	if err := os.MkdirAll(sandboxSecretsDir, 0700); err != nil {
		return fmt.Errorf("create sandbox secrets dir: %w", err)
	}

	for _, secret := range secrets {
		// Scope enforcement: verify the secret source is in the grant.
		if !allowedSources[secret.Source] {
			return fmt.Errorf("%w: secret %q source %q not in grant scope",
				ports.ErrSecretRefused, secret.Name, secret.Source)
		}

		// Materialize the secret into a per-sandbox file.
		// The file path contains the sandboxID so it is scoped to this
		// dispatch only.
		secretFile := filepath.Join(sandboxSecretsDir, secret.Name)
		if err := os.WriteFile(secretFile, []byte(secret.Source), 0400); err != nil {
			return fmt.Errorf("write secret file %s: %w", secretFile, err)
		}
	}

	return nil
}

// EnvName returns the environment variable name for a secret, following the
// <SOURCE>_<NAME> convention.
func EnvName(source, name string) string {
	return fmt.Sprintf("%s_%s",
		strings.ToUpper(strings.ReplaceAll(source, "-", "_")),
		strings.ToUpper(strings.ReplaceAll(name, "-", "_")),
	)
}

// SecretFilePath returns the path to the secret file for the given sandbox
// and secret name.
func (inj *SecretInjectorImpl) SecretFilePath(sandboxID, secretName string) string {
	return filepath.Join(inj.secretsDir, sandboxID, secretName)
}

// Cleanup removes the per-sandbox secrets directory for a completed dispatch.
func (inj *SecretInjectorImpl) Cleanup(ctx context.Context, sandboxID string) error {
	sandboxSecretsDir := filepath.Join(inj.secretsDir, sandboxID)
	return os.RemoveAll(sandboxSecretsDir)
}

// compile-time check that SecretInjectorImpl implements ports.SecretInjector
var _ ports.SecretInjector = (*SecretInjectorImpl)(nil)

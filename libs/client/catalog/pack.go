package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Pack zips a workspace directory into a catalog bundle. The directory tree is
// walked relative to dir, preserving paths (mework.yml at the root, nested
// .claude/ and other files), so the bundle round-trips back via ExtractWorkspace.
func Pack(dir string) ([]byte, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("pack %s: not a directory", dir)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		// Use forward slashes for zip entry names regardless of OS.
		name := filepath.ToSlash(rel)
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", dir, err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("pack %s: %w", dir, err)
	}
	return buf.Bytes(), nil
}

// ExtractWorkspace writes every file in a bundle zip into destDir, recreating
// the stored directory structure. It is the inverse of Pack.
func ExtractWorkspace(zipBytes []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return fmt.Errorf("invalid zip: %w", err)
	}
	for _, f := range zr.File {
		// Guard against zip-slip: reject entries that escape destDir.
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe bundle path %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open %s: %w", f.Name, err)
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %s: %w", target, err)
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return fmt.Errorf("extract %s: %w", f.Name, copyErr)
		}
	}
	return nil
}

// Push registers a bundle as an immutable version of the named agent/workspace
// via POST /api/v1/agents/{name}/versions. The bundle bytes are base64-encoded
// into the JSON publish request tagged form="bundle".
func Push(ctx context.Context, baseURL, name, version string, bundle []byte) error {
	body, err := json.Marshal(struct {
		Version string `json:"version"`
		Form    string `json:"form"`
		Payload string `json:"payload"`
	}{
		Version: version,
		Form:    "bundle",
		Payload: base64.StdEncoding.EncodeToString(bundle),
	})
	if err != nil {
		return err
	}

	url := strings.TrimRight(baseURL, "/") + "/api/v1/agents/" + name + "/versions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push %s@%s failed: %s: %s", name, version, resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

package catalog

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// makeZip builds a zip archive from a name->content map and returns its bytes.
func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// TestValidateBundle_AcceptsManifests covers the workspace manifest (mework.yml)
// being accepted alongside the legacy sandbox.yaml + definition.md form, and
// rejection when neither manifest is present (delta-spec "Pack then push").
func TestValidateBundle_AcceptsManifests(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "accepts mework.yml manifest",
			files: map[string]string{
				"mework.yml":            "name: my-workspace\nbackend: local\nengine: claude\n",
				".claude/settings.json": "{}",
				"README.md":             "# ws\n",
			},
			wantErr: false,
		},
		{
			name: "still accepts legacy sandbox.yaml + definition.md",
			files: map[string]string{
				"sandbox.yaml":  "name: code-fixer\nversion: v1\nspec: spec\nbackend: docker\n",
				"definition.md": "# Code Fixer\n",
			},
			wantErr: false,
		},
		{
			name: "rejects bundle with neither manifest",
			files: map[string]string{
				"README.md": "# nothing useful here\n",
			},
			wantErr:   true,
			errSubstr: "manifest",
		},
		{
			name: "mework.yml missing backend is rejected",
			files: map[string]string{
				"mework.yml": "name: my-workspace\n",
			},
			wantErr:   true,
			errSubstr: "backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBundle(makeZip(t, tt.files))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not mention %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

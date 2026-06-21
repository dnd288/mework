package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
)

// PackBundle creates a sandbox bundle zip from a source directory. It requires
// that the directory contains at least sandbox.yaml at the root.
func PackBundle(srcDir, outputPath string) (string, error) {
	info, err := os.Stat(srcDir)
	if err != nil {
		return "", fmt.Errorf("source directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", srcDir)
	}

	// Validate sandbox.yaml exists.
	sandboxYAML := filepath.Join(srcDir, "sandbox.yaml")
	if _, err := os.Stat(sandboxYAML); err != nil {
		return "", fmt.Errorf("bundle must contain sandbox.yaml: %w", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("create output file: %w", err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	err = filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		f, err := zw.Create(rel)
		if err != nil {
			return fmt.Errorf("create zip entry %s: %w", rel, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("write %s to zip: %w", rel, err)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk source directory: %w", err)
	}

	return outputPath, nil
}

package evaluationlab

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"strings"
)

// Inspect only direct files in the selector's own synthetic harness directory.
// Never follow stdout paths, Chrome profiles, nested caches or symlinks.
func captureProcessArtifacts(tmp string, command processCommand, limit int64, runtime string) ([]ProcessArtifact, error) {
	artifacts := []ProcessArtifact{}
	if command.artifactPrefix == "" {
		return artifacts, nil
	}
	root, err := os.OpenRoot(tmp)
	if err != nil {
		return artifacts, err
	}
	defer root.Close()
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) > 4096 {
		return artifacts, fmt.Errorf("temporary inventory exceeds bounds")
	}
	for _, directory := range entries {
		if !directory.IsDir() || !strings.HasPrefix(directory.Name(), command.artifactPrefix) {
			continue
		}
		files, readErr := os.ReadDir(filepath.Join(tmp, directory.Name()))
		if readErr != nil || len(files) > 4096 {
			return artifacts, fmt.Errorf("harness inventory unavailable")
		}
		for _, entry := range files {
			kind, allowed := command.artifactKinds[filepath.Ext(entry.Name())]
			if !allowed {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
				return artifacts, fmt.Errorf("unsafe artifact type")
			}
			info, statErr := entry.Info()
			if statErr != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 || info.Size() > limit || len(artifacts) >= 32 {
				return artifacts, fmt.Errorf("artifact byte/count limit")
			}
			file, openErr := root.Open(filepath.Join(directory.Name(), entry.Name()))
			if openErr != nil {
				return artifacts, openErr
			}
			raw, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
			file.Close()
			if readErr != nil || int64(len(raw)) > limit || len(raw) > 1<<20 {
				return artifacts, fmt.Errorf("artifact read bound")
			}
			if kind == "screenshot" && !bytes.HasPrefix(raw, []byte{137, 80, 78, 71, 13, 10, 26, 10}) || kind == "cdp-produced-pdf" && !bytes.HasPrefix(raw, []byte("%PDF-")) {
				return artifacts, fmt.Errorf("artifact format mismatch")
			}
			limit -= int64(len(raw))
			artifacts = append(artifacts, ProcessArtifact{Name: entry.Name(), Kind: kind, Origin: command.origin, Runtime: runtime, Hash: semanticeval.Hash(raw), Bytes: raw})
		}
	}
	return artifacts, nil
}

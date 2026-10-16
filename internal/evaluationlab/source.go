package evaluationlab

import (
	"fmt"
	"os"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"runtime/debug"
	"strings"
)

// CaptureSource reads only Go/embedded JSON in known implementation directories.
// The launcher's root is local configuration; browser/import data never supplies it.
func CaptureSource(root string) (Provenance, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Provenance{}, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return Provenance{}, err
	}
	p := Provenance{SourceHashes: map[string]string{}, UnavailableReasons: []string{}}
	modified := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				p.Commit = setting.Value
			}
			if setting.Key == "vcs.modified" {
				modified = setting.Value
			}
		}
	}
	if p.Commit == "" {
		p.UnavailableReasons = append(p.UnavailableReasons, "compiler VCS revision unavailable")
	}
	if modified == "" {
		p.UnavailableReasons = append(p.UnavailableReasons, "compiler clean/dirty state unavailable")
	}
	for _, directory := range []string{"backend", "internal", "scripts/semantic-quality", "scripts/workflow-settings", "scripts/qualification-evidence", "scripts/application-context"} {
		err := filepath.WalkDir(filepath.Join(canonical, directory), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("source snapshot rejects symlinks")
			}
			if entry.IsDir() || strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".json") {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() > MaxImportBytes {
				return fmt.Errorf("source file outside snapshot bounds")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(canonical, path)
			if err != nil {
				return err
			}
			p.SourceHashes[filepath.ToSlash(relative)] = semanticeval.Hash(raw)
			return nil
		})
		if err != nil {
			return Provenance{}, err
		}
	}
	if modified == "true" {
		p.DirtyHashes = map[string]string{}
		for path, hash := range p.SourceHashes {
			p.DirtyHashes[path] = hash
		}
	}
	p.UnavailableReasons = append(p.UnavailableReasons, "source snapshot is launch-file evidence; binary/source correspondence requires separate build evidence")
	return p, nil
}

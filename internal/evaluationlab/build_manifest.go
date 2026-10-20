package evaluationlab

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"runtime"
)

const BuildManifestVersion = "lab-build-manifest-v1"

// DependencyManifest states what an identity actually covers. An inline prompt
// source envelope is deliberately not an exact rendered template identity.
type DependencyManifest struct {
	Kind         string            `json:"kind"`
	Hash         string            `json:"hash"`
	Dependencies map[string]string `json:"dependencies"`
}

type VariantBuildManifest struct {
	ID             string             `json:"id"`
	Implementation DependencyManifest `json:"implementation"`
	Prompt         DependencyManifest `json:"prompt"`
	Schema         DependencyManifest `json:"schema"`
	Strategy       DependencyManifest `json:"strategy"`
	Scope          string             `json:"scope"`
}

type BuildManifest struct {
	Version           string                 `json:"version"`
	Hash              string                 `json:"hash"`
	Runtime           string                 `json:"runtime"`
	Sources           map[string]string      `json:"sources"`
	TemplateArtifacts DependencyManifest     `json:"templateArtifacts"`
	Variants          []VariantBuildManifest `json:"variants"`
}

// CaptureBuildManifest reads owned build inputs, never browser-provided paths.
// The conservative closure includes all captured backend/internal code, embedded
// JSON, the launcher, module locks and UI inputs; it makes no isolated prompt
// attribution claim. Fixture identities retain their separate existing fields.
func CaptureBuildManifest(root string) (BuildManifest, error) {
	provenance, err := CaptureSource(root)
	if err != nil {
		return BuildManifest{}, err
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return BuildManifest{}, err
	}
	sources := provenance.SourceHashes
	for _, name := range []string{"go.mod", "go.sum", "package.json", "package-lock.json", "lab/buildmanifest/main.go"} {
		info, err := os.Lstat(filepath.Join(canonical, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() > MaxImportBytes {
			return BuildManifest{}, fmt.Errorf("build dependency unavailable or unsafe: %s", name)
		}
		raw, err := os.ReadFile(filepath.Join(canonical, name))
		if err != nil {
			return BuildManifest{}, err
		}
		sources[name] = semanticeval.Hash(raw)
	}
	manifest := BuildManifest{Version: BuildManifestVersion, Runtime: runtime.Version() + "/" + runtime.GOOS + "/" + runtime.GOARCH, Sources: sources, Variants: []VariantBuildManifest{}}
	if !evidenceHash.MatchString(sources["internal/fieldvalidation/questions.json"]) {
		return BuildManifest{}, fmt.Errorf("explicit classification template artifact unavailable")
	}
	manifest.TemplateArtifacts = dependencyManifest("explicit-manifest", "template-artifacts", map[string]string{"internal/fieldvalidation/questions.json": sources["internal/fieldvalidation/questions.json"]})
	envelope := map[string]string{"build-source-set": versionedHash("lab-build-source-set-v1", sources)}
	for _, registration := range variantRegistry {
		scope := "conservative co-built adapter dependencies; inline templates, schema builders and strategy are source envelopes; rendered per-call hashes remain separate"
		if registration.suiteID == "test-adapters" {
			scope += "; subprocess test/tool execution is not attested by the launcher build"
		}
		manifest.Variants = append(manifest.Variants, VariantBuildManifest{ID: registration.id, Implementation: dependencyManifest("source-envelope", "implementation", envelope), Prompt: dependencyManifest("source-envelope", "prompt", envelope), Schema: dependencyManifest("source-envelope", "producer-schema", envelope), Strategy: dependencyManifest("source-envelope", "strategy", envelope), Scope: scope})
	}
	manifest.Hash = buildManifestHash(manifest)
	return manifest, nil
}

func dependencyManifest(kind, domain string, sources map[string]string) DependencyManifest {
	return DependencyManifest{Kind: kind, Hash: versionedHash("lab-"+domain+"-"+kind+"-v1", sources), Dependencies: cloneJSON(sources)}
}

func buildManifestHash(manifest BuildManifest) string {
	manifest.Hash = ""
	raw, _ := json.Marshal(manifest)
	return semanticeval.Hash(raw)
}

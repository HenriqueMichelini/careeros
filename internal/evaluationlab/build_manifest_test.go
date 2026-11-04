package evaluationlab_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	eval "professional-information-repo/internal/semanticeval"
	"strings"
	"testing"
)

func manifestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"backend/profile-ingestion", "internal/fieldvalidation", "lab/api", "lab/ui", "lab/process", "lab/buildmanifest", "scripts/semantic-quality", "scripts/workflow-settings", "scripts/qualification-evidence", "scripts/application-context"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, value := range map[string]string{"backend/profile-ingestion/main.go": "package ingestion\n// instruction template alpha", "internal/fieldvalidation/questions.json": `{"instruction":"classify"}`, "vite.lab.config.ts": "export default {}", "scripts/lab.mjs": "// launcher", "lab/buildmanifest/main.go": "package main", "go.mod": "module example\ngo 1.24", "go.sum": "module checksum", "package.json": "{}", "package-lock.json": "{}"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCaptureBuildManifestTracksTemplateDependenciesWithoutClaimingExactPromptIdentity(t *testing.T) {
	root := manifestTree(t)
	before, err := lab.CaptureBuildManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if before.Hash == "" || len(before.Variants) != 8 {
		t.Fatalf("incomplete registered build: %+v", before)
	}
	for _, variant := range before.Variants {
		if variant.Prompt.Kind != "source-envelope" || variant.Schema.Kind != "source-envelope" || variant.Strategy.Kind != "source-envelope" {
			t.Fatalf("inline source mislabeled: %+v", variant)
		}
		if variant.Prompt.Hash == "" || len(variant.Prompt.Dependencies) == 0 {
			t.Fatalf("missing dependencies: %+v", variant)
		}
	}
	if before.TemplateArtifacts.Kind != "explicit-manifest" || before.TemplateArtifacts.Dependencies["internal/fieldvalidation/questions.json"] == "" {
		t.Fatal("explicit rubric artifact unavailable")
	}
	if err := os.WriteFile(filepath.Join(root, "backend/profile-ingestion/main.go"), []byte("package ingestion\n// instruction template beta"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := lab.CaptureBuildManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if before.Hash == after.Hash || before.Variants[0].Prompt.Hash == after.Variants[0].Prompt.Hash {
		t.Fatal("changed template source retained build/prompt envelope identity")
	}
}

func TestCheckBuildCorrespondenceRejectsChangedBuildEvidenceAndKeepsAbsenceUnavailable(t *testing.T) {
	root := manifestTree(t)
	compiled, err := lab.CaptureBuildManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	executable := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	receipt := lab.BuildReceipt{Version: lab.BuildReceiptVersion, ManifestHash: compiled.Hash, ExecutableHash: executable, LauncherHash: compiled.Sources["scripts/lab.mjs"], Runtime: compiled.Runtime, Command: lab.BuildCommand()}
	if got := lab.CheckBuildCorrespondence(compiled, compiled, receipt, executable); got.Status != "verified" {
		t.Fatalf("co-built evidence not verified: %+v", got)
	}
	if got := lab.CheckBuildCorrespondence(lab.BuildManifest{}, compiled, lab.BuildReceipt{}, executable); got.Status != "unavailable" {
		t.Fatalf("old launcher given authority: %+v", got)
	}
	if got := lab.CheckBuildCorrespondence(compiled, compiled, lab.BuildReceipt{}, executable); got.Status != "unavailable" {
		t.Fatalf("missing receipt given authority: %+v", got)
	}
	for _, change := range []string{"source", "receipt", "binary", "compiled", "launcher", "command", "runtime"} {
		t.Run(change, func(t *testing.T) {
			launch := compiled
			proof := receipt
			binary := executable
			embedded := compiled
			switch change {
			case "source":
				if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte("changed dependency checksum"), 0600); err != nil {
					t.Fatal(err)
				}
				launch, err = lab.CaptureBuildManifest(root)
				if err != nil {
					t.Fatal(err)
				}
			case "receipt":
				proof.ManifestHash = executable
			case "binary":
				binary = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			case "compiled":
				embedded.Hash = executable
			case "launcher":
				proof.LauncherHash = executable
			case "command":
				proof.Command = []string{"go", "run", "unowned"}
			case "runtime":
				proof.Runtime = "different-toolchain"
			}
			if got := lab.CheckBuildCorrespondence(embedded, launch, proof, binary); got.Status != "mismatch" {
				t.Fatalf("changed %s accepted: %+v", change, got)
			}
		})
	}
}

func TestCheckBuildCorrespondenceDoesNotLetMissingEvidenceMaskKnownConflicts(t *testing.T) {
	manifest, err := lab.CaptureBuildManifest(manifestTree(t))
	if err != nil {
		t.Fatal(err)
	}
	executable := strings.Repeat("a", 64)
	receipt := lab.BuildReceipt{Version: lab.BuildReceiptVersion, ManifestHash: manifest.Hash, ExecutableHash: executable, LauncherHash: manifest.Sources["scripts/lab.mjs"], Runtime: manifest.Runtime, Command: lab.BuildCommand()}
	type evidence struct {
		compiled, launch lab.BuildManifest
		receipt          lab.BuildReceipt
		digest           string
	}
	type change struct {
		name  string
		apply func(*evidence)
	}
	missing := []change{
		{"receipt version", func(e *evidence) { e.receipt.Version = "" }},
		{"manifest hash", func(e *evidence) { e.receipt.ManifestHash = "" }},
		{"executable hash", func(e *evidence) { e.receipt.ExecutableHash = "" }},
		{"launcher hash", func(e *evidence) { e.receipt.LauncherHash = "" }},
		{"toolchain", func(e *evidence) { e.receipt.Runtime = "" }},
		{"recipe", func(e *evidence) { e.receipt.Command = nil }},
		{"compiled manifest", func(e *evidence) { e.compiled = lab.BuildManifest{} }},
		{"compiled schema", func(e *evidence) { e.compiled.Version = "" }},
		{"compiled hash", func(e *evidence) { e.compiled.Hash = "" }},
		{"launch manifest", func(e *evidence) { e.launch = lab.BuildManifest{} }},
		{"launch schema", func(e *evidence) { e.launch.Version = "" }},
		{"launch hash", func(e *evidence) { e.launch.Hash = "" }},
		{"executing digest", func(e *evidence) { e.digest = "" }},
	}
	conflicts := []change{
		{"receipt version", func(e *evidence) { e.receipt.Version = "unsupported-receipt" }},
		{"manifest hash", func(e *evidence) { e.receipt.ManifestHash = strings.Repeat("b", 64) }},
		{"executable hash", func(e *evidence) { e.receipt.ExecutableHash = strings.Repeat("b", 64) }},
		{"launcher hash", func(e *evidence) { e.receipt.LauncherHash = strings.Repeat("b", 64) }},
		{"toolchain", func(e *evidence) { e.receipt.Runtime = "different-toolchain" }},
		{"recipe", func(e *evidence) { e.receipt.Command = []string{"go", "run", "unowned"} }},
		{"malformed receipt manifest hash", func(e *evidence) { e.receipt.ManifestHash = "invalid" }},
		{"malformed receipt executable hash", func(e *evidence) { e.receipt.ExecutableHash = "invalid" }},
		{"malformed receipt launcher hash", func(e *evidence) { e.receipt.LauncherHash = "invalid" }},
		{"malformed executing digest", func(e *evidence) { e.digest = "invalid" }},
	}
	for _, absent := range missing {
		t.Run(absent.name, func(t *testing.T) {
			makeEvidence := func() evidence {
				e := evidence{compiled: manifest, launch: manifest, receipt: receipt, digest: executable}
				absent.apply(&e)
				return e
			}
			e := makeEvidence()
			if got := lab.CheckBuildCorrespondence(e.compiled, e.launch, e.receipt, e.digest); got.Status != "unavailable" {
				t.Fatalf("absence alone must remain unavailable: %+v", got)
			}
			for _, conflict := range conflicts {
				t.Run(conflict.name, func(t *testing.T) {
					e := makeEvidence()
					conflict.apply(&e)
					want := "mismatch"
					if absent.name == "executing digest" && conflict.name == "executable hash" {
						want = "unavailable" // No actual digest remains to contradict this well-formed claim.
					}
					if got := lab.CheckBuildCorrespondence(e.compiled, e.launch, e.receipt, e.digest); got.Status != want {
						t.Fatalf("missing %s with %s: want %s, got %+v", absent.name, conflict.name, want, got)
					}
				})
			}
		})
	}
	for _, absent := range []change{
		{"receipt", func(e *evidence) { e.receipt = lab.BuildReceipt{} }},
		{"compiled manifest", func(e *evidence) { e.compiled = lab.BuildManifest{} }},
	} {
		for _, conflict := range []change{
			{"launch schema", func(e *evidence) { e.launch.Version = "unsupported-manifest" }},
			{"launch hash", func(e *evidence) { e.launch.Hash = strings.Repeat("b", 64) }},
			{"malformed launch hash", func(e *evidence) { e.launch.Hash = "invalid" }},
		} {
			t.Run(absent.name+" with "+conflict.name, func(t *testing.T) {
				e := evidence{compiled: manifest, launch: manifest, receipt: receipt, digest: executable}
				absent.apply(&e)
				conflict.apply(&e)
				if got := lab.CheckBuildCorrespondence(e.compiled, e.launch, e.receipt, e.digest); got.Status != "mismatch" {
					t.Fatalf("missing evidence masked invalid launch manifest: %+v", got)
				}
			})
		}
	}
}

func TestResolvedVariantRetainsBuildEvidenceAndRejectsKnownMismatch(t *testing.T) {
	manifest, err := lab.CaptureBuildManifest(manifestTree(t))
	if err != nil {
		t.Fatal(err)
	}
	binary := strings.Repeat("a", 64)
	receipt := lab.BuildReceipt{Version: lab.BuildReceiptVersion, ManifestHash: manifest.Hash, ExecutableHash: binary, LauncherHash: manifest.Sources["scripts/lab.mjs"], Runtime: manifest.Runtime, Command: lab.BuildCommand()}
	proof := lab.CheckBuildCorrespondence(manifest, manifest, receipt, binary)
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string][]byte{"semantic-quality": raw}
	mismatch := proof
	mismatch.Status = "mismatch"
	if _, err := lab.NewPlanner(assets, lab.Provenance{Build: &mismatch}); err == nil {
		t.Fatal("known build mismatch admitted to planner")
	}
	source := lab.Provenance{Commit: "original-commit-meaning", SourceHashes: map[string]string{"captured-source": "original-source-hash"}, Build: &proof, UnavailableReasons: []string{"original provenance limitation"}}
	planner, err := lab.NewPlanner(assets, source)
	if err != nil {
		t.Fatal(err)
	}
	proof.Manifest.Hash = "caller mutation"
	variant, err := lab.ResolveVariant("semantic-quality", "semantic-quality-native-v1", "current")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := variant.Plan(planner, []string{"en-fact", "pt-fact"}, lab.ExecutionCaps{MaxCases: 2, MaxHTTPAttempts: 2, MaxProviderCalls: 10, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Provenance.Build.Status != "verified" || plan.Provenance.Build.Manifest.Hash != manifest.Hash || plan.Provenance.Commit != "original-commit-meaning" || plan.Provenance.SourceHashes["captured-source"] != "original-source-hash" || plan.Provenance.UnavailableReasons[0] != "original provenance limitation" {
		t.Fatal("build references displaced or borrowed prior provenance")
	}
	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	id, err := variant.Start(context.Background(), runner, plan)
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, runner, id)
	if record.Provenance.Build.Manifest.Hash != manifest.Hash || !strings.HasPrefix(record.Context.PromptVersion, "lab-prompt-source-envelope-v1:") || !strings.HasPrefix(record.Context.StrategyVersion, "lab-strategy-source-envelope-v1:") {
		t.Fatalf("old envelope meanings changed: %+v", record.Context)
	}
	var first, second eval.RunResult
	if err := json.Unmarshal(record.Cases[0].Actual, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record.Cases[1].Actual, &second); err != nil {
		t.Fatal(err)
	}
	observed := false
	for _, a := range first.Calls {
		for _, b := range second.Calls {
			if a.InstructionPrefixHash != "" && a.InstructionPrefixHash == b.InstructionPrefixHash && a.PromptHash != b.PromptHash && a.SchemaHash != "" && a.RequestHash != b.RequestHash {
				observed = true
			}
		}
	}
	if !observed {
		t.Fatal("controlled rendered-call evidence did not distinguish equal instruction prefixes from different complete prompts")
	}
}

func TestCaptureBuildManifestRequiresItsExplicitTemplateArtifact(t *testing.T) {
	root := manifestTree(t)
	if err := os.Remove(filepath.Join(root, "internal/fieldvalidation/questions.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := lab.CaptureBuildManifest(root); err == nil {
		t.Fatal("missing artifact certified as an explicit manifest")
	}
}

func TestCaptureBuildManifestChangesWithSchemaStrategyLauncherAndModuleInputs(t *testing.T) {
	for _, path := range []string{"backend/profile-ingestion/schema.go", "internal/semanticeval/configuration.go", "internal/fieldvalidation/questions.json", "go.mod", "go.sum", "scripts/lab.mjs", "lab/buildmanifest/main.go"} {
		t.Run(path, func(t *testing.T) {
			root := manifestTree(t)
			full := filepath.Join(root, path)
			if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte("original dependency"), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := lab.CaptureBuildManifest(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte("changed dependency"), 0600); err != nil {
				t.Fatal(err)
			}
			after, err := lab.CaptureBuildManifest(root)
			if err != nil {
				t.Fatal(err)
			}
			if before.Hash == after.Hash || before.Variants[0].Schema.Hash == after.Variants[0].Schema.Hash || before.Variants[0].Strategy.Hash == after.Variants[0].Strategy.Hash {
				t.Fatal("changed dependency silently retained co-built identities")
			}
			if path == "internal/fieldvalidation/questions.json" && before.TemplateArtifacts.Hash == after.TemplateArtifacts.Hash {
				t.Fatal("changed explicit template kept identity")
			}
		})
	}
}

func TestNativeCatalogDoesNotClaimCompiledIdentityWithoutBuildEvidence(t *testing.T) {
	for _, suite := range lab.Catalog() {
		for _, descriptor := range suite.Variants {
			if descriptor.BuildManifest == nil || descriptor.BuildManifest.Correspondence != "unavailable" || descriptor.BuildManifest.UnavailableReason == "" || descriptor.BuildManifest.Hash != "" {
				t.Fatalf("ordinary Go build did not disclose missing compiled identity: %+v", descriptor)
			}
		}
	}
}

package evaluationlab

type processCommand struct {
	tool, origin   string
	unavailable    string
	args           []string
	browser        bool
	requiresGo     bool
	artifactPrefix string
	artifactKinds  map[string]string
}

// Every executable/package/file selector is server-owned. Browser data cannot
// provide command text, argument arrays, paths, environments or provider hosts.
var processCommands = map[string]processCommand{
	"probe-assertion":           {tool: "node", origin: "isolated-guard-probe", args: []string{"lab/process/probes.mjs", "assertion"}},
	"go-canonical":              {tool: "go", origin: "isolated-go-tests", args: []string{"test", "-count=1", "./internal/profiledocument"}},
	"go-ingestion":              {tool: "go", origin: "isolated-go-tests", args: []string{"test", "-count=1", "./backend/profile-ingestion", "./backend/profile-review"}},
	"go-job-application":        {tool: "go", origin: "isolated-go-tests", args: []string{"test", "-count=1", "./backend/qualification-gaps", "./backend/application-draft"}},
	"go-cv":                     {tool: "go", origin: "isolated-go-tests", args: []string{"test", "-count=1", "./backend/cv-generation"}},
	"node-ingestion":            {tool: "node", origin: "isolated-node-tests", args: []string{"--test", "--test-concurrency=1", "--test-reporter=tap", "tests/ingestion.test.mjs"}},
	"node-job-reuse":            {tool: "node", origin: "isolated-node-tests", args: []string{"--test", "--test-concurrency=1", "--test-reporter=tap", "tests/qualification-evidence.test.mjs", "tests/contact-profile.test.mjs"}},
	"node-artifact-review":      {tool: "node", origin: "isolated-node-tests", args: []string{"--test", "--test-concurrency=1", "--test-reporter=tap", "tests/cv-choices.test.mjs", "tests/cv-preferences.test.mjs", "tests/cv-generation.test.mjs", "tests/resume-review.test.mjs", "tests/artifact-review.test.mjs"}, requiresGo: true},
	"browser-canonical-edit":    {tool: "node", origin: "isolated-browser-harness", args: []string{"--import", "./lab/process/browser-diagnostics.mjs", "tests/profile-edit-browser.check.mjs"}, browser: true, artifactPrefix: "careeros-manual-browser-", artifactKinds: map[string]string{".png": "screenshot"}},
	"browser-ingestion":         {tool: "node", origin: "isolated-browser-harness", args: []string{"--import", "./lab/process/browser-diagnostics.mjs", "tests/ingestion-browser.check.mjs"}, browser: true, artifactPrefix: "careeros-ingestion-browser-", artifactKinds: map[string]string{".png": "screenshot"}},
	"browser-enrichment":        {tool: "node", origin: "isolated-browser-harness", args: []string{"--import", "./lab/process/browser-diagnostics.mjs", "tests/apply-checklist.check.mjs"}, browser: true, artifactPrefix: "careeros-apply-checklist-", artifactKinds: map[string]string{".png": "screenshot"}},
	"browser-application-reuse": {tool: "node", origin: "isolated-browser-harness", args: []string{"--import", "./lab/process/browser-diagnostics.mjs", "tests/application-reuse-browser.check.mjs"}, browser: true, artifactPrefix: "careeros-application-reuse-", artifactKinds: map[string]string{".png": "screenshot"}},
	"browser-resume-review":     {tool: "node", origin: "isolated-browser-harness", args: []string{"--import", "./lab/process/browser-diagnostics.mjs", "tests/resume-review-browser.check.mjs"}, browser: true, artifactPrefix: "careeros-resume-review-", artifactKinds: map[string]string{".png": "screenshot", ".pdf": "cdp-produced-pdf"}},
	"node-field-tooling":        {tool: "node", origin: "isolated-node-tests", args: []string{"--test", "--test-reporter=tap", "tests/field-validation.test.mjs"}},
	"native-print-capability":   {origin: "native-print-capability", unavailable: "native_print_unavailable"},
	"probe-crash":               {tool: "node", origin: "isolated-guard-probe", args: []string{"lab/process/probes.mjs", "crash"}},
	"probe-tool-missing":        {tool: "unavailable-test-runtime", origin: "isolated-guard-probe"},
	"go-preprocessing":          {tool: "go", origin: "isolated-go-tests", args: []string{"test", "-count=1", "./internal/preprocessing"}},
	"node-profile-document":     {tool: "node", origin: "isolated-node-tests", args: []string{"--test", "--test-reporter=tap", "tests/profile-document.test.mjs"}},
	"browser-section-review":    {tool: "node", origin: "isolated-browser-harness", args: []string{"--import", "./lab/process/browser-diagnostics.mjs", "tests/section-review-browser.check.mjs"}, browser: true, artifactPrefix: "careeros-section-browser-", artifactKinds: map[string]string{".png": "screenshot"}},
	"probe-isolation":           {tool: "node", origin: "isolated-guard-probe", args: []string{"lab/process/probes.mjs", "isolation"}},
	"probe-hang-tree":           {tool: "node", origin: "isolated-guard-probe", args: []string{"lab/process/probes.mjs", "hang-tree"}},
	"probe-output":              {tool: "node", origin: "isolated-guard-probe", args: []string{"lab/process/probes.mjs", "output"}},
}

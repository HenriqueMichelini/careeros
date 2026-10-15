package evaluationlab

// Exact approved synthetic evidence, not permission to execute.
type retainedArtifact struct{ Path, Version string }

var retainedArtifacts = map[string]retainedArtifact{
	"5d231ea5c263ae88e4b53aa4c677d56a2e03b2889caa50764e27bbc97a2617ef": {Path: "docs/evaluations/semantic-quality/controlled.v1.json", Version: "semantic-quality-report-v1"},
	"54dcbc2d520d9f896a06aa838bce9d11386ac78b01a62875221f0692a82e1d17": {Path: "docs/evaluations/semantic-quality/controlled.v2.json", Version: "semantic-quality-report-v2"},
	"48367e321c140fc4f942d2ecf81ba7117f420adf608af7d11ce6c439ffbc42b0": {Path: "docs/evaluations/semantic-quality/live.final.v1.json", Version: "semantic-quality-report-v1"},
	"e2cf07752fb6a7717647005aaabc57aa5473b1aee671b40f1c26eb4b3616480b": {Path: "docs/evaluations/semantic-quality/live.initial.v1.json", Version: "semantic-quality-report-v1"},
	"c98f395d2b76118cc55651f1ee83fe331e2683406880edd258861fab65a24d31": {Path: "docs/evaluations/semantic-quality/live.remaining.v1.json", Version: "semantic-quality-report-v1"},
	"c61099b2e96fcc3790eec04d23ad7921c8044a93e4642c8253b6e866459446b6": {Path: "docs/evaluations/semantic-quality/live.replay.v2.json", Version: "semantic-quality-report-v2"},
	"46223f47959ce0e16f4c39b0b4c58bc376161ddbe3ac4b4ebb30e13e07a8d854": {Path: "docs/evaluations/workflow-settings/controlled.v1.json", Version: "workflow-settings-v1"},
	"f93f658ea837c0fd52283ad40ebe4fa69ae315c790dc5f1b159a581535f905e9": {Path: "docs/evaluations/workflow-settings/live.v1.json", Version: "workflow-settings-v1"},
	"81d6ff01ddfd258c0469bd091a682c74f2f5d08a0750dce9a1c8f4b7a61fd980": {Path: "docs/evaluations/workflow-settings/reviewed.v1.json", Version: "workflow-settings-v1"},
	"cf6765f0a690d790720ef6f8efd37a8a119f33fd6913804477dd5ccc535385ac": {Path: "docs/evaluations/ai-diagnostics.controlled.v1.json", Version: ""},
	"807513abbe3e83dcd41cfecbf35fd82bfa2e083c5788fa440bda5cce242d4513": {Path: "docs/evaluations/qualification-evidence/controlled-report.json", Version: ""},
	"df93e2200d0346cf4d76a33d22f80d15e94e634b98a3f6d208bd661849cc26f5": {Path: "docs/evaluations/application-context/controlled-report.json", Version: ""},
}

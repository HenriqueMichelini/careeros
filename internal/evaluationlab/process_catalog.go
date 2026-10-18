// Static catalog bound to the reviewed source/selector manifest.
package evaluationlab

const processFixtureHash = "c3fb7be93b892adca689c9798370d89bac305454ae3268055488f25376e7c595"

func init() {
	suiteCatalog = append(suiteCatalog, Suite{
		ID:                 "test-adapters",
		Version:            "lab-suite-v1",
		FixtureHash:        processFixtureHash,
		Modes:              []string{"offline"},
		Configurations:     []string{"current"},
		RequiredTools:      []string{"go", "node", "google-chrome", "bubblewrap"},
		Sensitivity:        "approved-synthetic",
		AssertionSemantics: "fixed isolated test exit assertions only; negative guard probes deliberately stop/fail; no semantic quality, user acceptance or provider claim",
		Cases: []FixtureCase{
			{ID: "go-preprocessing", Hash: "6bdde2c6a1b3307fc65dc78e8e7ade9861ab0fb25bd78fcbaf81fd0131f004b8", Workflow: "preprocessing_contracts", Language: "language-independent", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "node-profile-document", Hash: "061336db7d9e3de735e130dcc3dcd1c37ee52d39a350c71c8ea08a0140721942", Workflow: "canonical_profile_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "browser-section-review", Hash: "b37d2cbc57472e969e83ad0945ca6dc7d10e73230da33f995c51bcd20fbcc349", Workflow: "section_review_browser_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "probe-isolation", Hash: "63a9832f083cf37df794ba9b79321939db161426a9de9bf49bc866263675652f", Workflow: "lab_isolation_contracts", Language: "language-independent", Split: "development", ExposureGroup: "guard-probes", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "probe-hang-tree", Hash: "84987838a096af7f17472badf3c4d7da03b4317d96eaff277b43407d47c649b1", Workflow: "lab_process_lifecycle_contracts", Language: "language-independent", Split: "development", ExposureGroup: "guard-probes", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "probe-output", Hash: "5383fd80e30cccbba8ca0d49e87660a41c2d85ec5e6d7af1975e01a5df96b144", Workflow: "lab_process_lifecycle_contracts", Language: "language-independent", Split: "development", ExposureGroup: "guard-probes", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "probe-tool-missing", Hash: "7dd843f828eaba475a142a5d10c68dd4e5849e252e642c9430b96882e3922041", Workflow: "lab_process_environment_contracts", Language: "language-independent", Split: "development", ExposureGroup: "guard-probes", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "probe-crash", Hash: "b0b321bace241236453b5f426d0817dbd74767a3fded6b4161469bcad1f9947a", Workflow: "lab_process_environment_contracts", Language: "language-independent", Split: "development", ExposureGroup: "guard-probes", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "native-print-capability", Hash: "85298b487760942011001a4499119ef253fb0b258dfbf9e60a0580d081c493db", Workflow: "native_export_capability", Language: "language-independent", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "node-field-tooling", Hash: "40b93134d0a56c3ca571d9cec705643a5a6076077a6cf9009c3ab29c820d5069", Workflow: "offline_field_tooling_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "go-canonical", Hash: "5aede83a9aef3b5571733e3728c9afe452f095941d15ce4fb6c5fcd8a4bb9d17", Workflow: "canonical_profile_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "go-ingestion", Hash: "fe01597babae3718806319ed2bb002e400b9d9341bfe1c055b4fbaf7263b9f45", Workflow: "ingestion_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "go-job-application", Hash: "fc15e0a682a97076ec9459351692c194cd66aea97c20b13d644c0e4a0ec67e71", Workflow: "job_application_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "go-cv", Hash: "5dafb3d1c20583b244b0c751512252694254a2185240510881a6b9777ff4b19f", Workflow: "cv_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "node-ingestion", Hash: "0de48e17b8ec5fc251fd15a1037751fb6c2b0d941b13f45cebf146dfed59ff97", Workflow: "ingestion_client_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "node-job-reuse", Hash: "3e6f8de7ff4aae7e34aa8f0ba96731772e56e348ee22707ef576e4937cae42d8", Workflow: "job_reuse_client_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "node-artifact-review", Hash: "e3dfdd2317f74f4c85031b7d08f1b93073a5a4401de8190159b00d354cf699a7", Workflow: "artifact_review_client_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "browser-canonical-edit", Hash: "1ad91f036a9894cbb76257a77d5ee94670d05dc256e9a5043894a73456e9cfc3", Workflow: "canonical_profile_browser_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "browser-ingestion", Hash: "0ca6c5b6611e5863b0b6c7107525c6fff973f053d4203d3faeb157dc0a34557b", Workflow: "ingestion_browser_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "browser-enrichment", Hash: "f999981714465a7252bd9a78e1a95cfb0b1eb08141b9eab7ef58562dddfbc7ce", Workflow: "apply_enrichment_browser_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "browser-application-reuse", Hash: "2039fc8c38731464c8b09eec692758bc29474a9faf2109272ef57cea210d5329", Workflow: "application_reuse_browser_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "browser-resume-review", Hash: "e0a0e5d71f61b7eb96eca3f5c4f60256f419cfcca3d7a29bc561bda7d19e4429", Workflow: "resume_review_export_browser_contracts", Language: "en+pt-BR", Split: "development", ExposureGroup: "existing-tests", RequiredSteps: 0, MaximumCalls: 0},
			{ID: "probe-assertion", Hash: "f905703649fdd55853d6a0699812fa8e3652d1905db987c9ede7fbdd7e3e8595", Workflow: "lab_process_assertion_contracts", Language: "language-independent", Split: "development", ExposureGroup: "guard-probes", RequiredSteps: 0, MaximumCalls: 0},
		},
	})
}

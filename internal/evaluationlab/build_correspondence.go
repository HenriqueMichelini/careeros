package evaluationlab

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
)

const BuildReceiptVersion = "lab-build-receipt-v1"

// Populated only by the local owned build command. Empty in ordinary go builds.
var compiledBuildManifest string

type BuildReceipt struct {
	Version        string   `json:"version"`
	ManifestHash   string   `json:"manifestHash"`
	ExecutableHash string   `json:"executableHash"`
	LauncherHash   string   `json:"launcherHash"`
	Runtime        string   `json:"runtime"`
	Command        []string `json:"command"`
}

type BuildCorrespondence struct {
	Version        string         `json:"version"`
	Status         string         `json:"status"`
	Manifest       *BuildManifest `json:"manifest,omitempty"`
	Receipt        *BuildReceipt  `json:"receipt,omitempty"`
	ExecutableHash string         `json:"executableHash,omitempty"`
	Reasons        []string       `json:"reasons"`
}

// BuildCommand records the fixed recipe. The variable linker value is the full
// base64 JSON identified by ManifestHash, not a user-selectable flag or command.
func BuildCommand() []string {
	return []string{"go", "build", "-trimpath", "-buildvcs=true", "-ldflags=-X=professional-information-repo/internal/evaluationlab.compiledBuildManifest=<base64-manifest>", "-o", "lab/.bin/lab-api", "./lab/api"}
}

func CompiledBuildManifest() (BuildManifest, error) {
	if compiledBuildManifest == "" {
		return BuildManifest{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(compiledBuildManifest)
	if err != nil {
		return BuildManifest{}, err
	}
	var manifest BuildManifest
	err = json.Unmarshal(raw, &manifest)
	return manifest, err
}

// CheckBuildCorrespondence verifies a trusted local builder's correspondence
// claim. It is neither a signed attestation nor a reproducible-build proof.
// Absent legacy evidence remains unavailable; supplied conflicting evidence is
// a mismatch. It never upgrades launch-file hashes alone to execution identity.
func CheckBuildCorrespondence(compiled, launch BuildManifest, receipt BuildReceipt, executingDigest string) BuildCorrespondence {
	result := BuildCorrespondence{Version: "lab-build-correspondence-v1", Status: "unavailable", Reasons: []string{}}
	if compiled.Version != "" {
		result.Manifest = ptrBuildManifest(cloneJSON(compiled))
	}
	if receipt.Version != "" || receipt.ManifestHash != "" || receipt.ExecutableHash != "" || receipt.LauncherHash != "" || receipt.Runtime != "" || len(receipt.Command) != 0 {
		ownedReceipt := cloneJSON(receipt)
		result.Receipt = &ownedReceipt
	}
	result.ExecutableHash = executingDigest
	// Check every independently supplied claim before classifying missing evidence.
	// In particular, absent schema/digest evidence cannot hide a known conflict.
	invalidHash := func(hash string) bool { return hash != "" && !evidenceHash.MatchString(hash) }
	conflicts := func(a, b string) bool { return a != "" && b != "" && a != b }
	for _, manifest := range []BuildManifest{compiled, launch} {
		if manifest.Version != "" && manifest.Version != BuildManifestVersion || invalidHash(manifest.Hash) || manifest.Version == BuildManifestVersion && manifest.Hash != "" && manifest.Hash != buildManifestHash(manifest) {
			result.Status = "mismatch"
			result.Reasons = append(result.Reasons, "compiled manifest or launch dependencies are invalid")
			return result
		}
		if conflicts(receipt.ManifestHash, manifest.Hash) || conflicts(receipt.LauncherHash, manifest.Sources["scripts/lab.mjs"]) || conflicts(receipt.Runtime, manifest.Runtime) {
			result.Status = "mismatch"
			result.Reasons = append(result.Reasons, "launcher receipt does not match supplied manifest, launcher or toolchain")
			return result
		}
	}
	if conflicts(compiled.Hash, launch.Hash) || receipt.Version != "" && receipt.Version != BuildReceiptVersion || invalidHash(executingDigest) || invalidHash(receipt.ManifestHash) || invalidHash(receipt.ExecutableHash) || invalidHash(receipt.LauncherHash) || conflicts(receipt.ExecutableHash, executingDigest) || len(receipt.Command) != 0 && !reflect.DeepEqual(receipt.Command, BuildCommand()) {
		result.Status = "mismatch"
		result.Reasons = append(result.Reasons, "compiled manifest, launcher receipt, executable or build recipe conflict")
		return result
	}
	if compiled.Version == "" || compiled.Hash == "" || launch.Version == "" || launch.Hash == "" || compiled.Runtime == "" || compiled.Sources["scripts/lab.mjs"] == "" {
		result.Reasons = append(result.Reasons, "compiled manifest or launch dependencies unavailable")
		return result
	}
	if receipt.Version == "" || receipt.ManifestHash == "" || receipt.ExecutableHash == "" || receipt.LauncherHash == "" || receipt.Runtime == "" || len(receipt.Command) == 0 || executingDigest == "" {
		result.Reasons = append(result.Reasons, "launcher receipt or executing binary digest incomplete or unavailable")
		return result
	}
	result.Status = "verified"
	return result
}

func ptrBuildManifest(manifest BuildManifest) *BuildManifest { return &manifest }

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
	if compiled.Version == "" {
		result.Reasons = append(result.Reasons, "compiled manifest unavailable")
		return result
	}
	result.Manifest = ptrBuildManifest(cloneJSON(compiled))
	if compiled.Version != BuildManifestVersion || compiled.Hash != buildManifestHash(compiled) || launch.Version != BuildManifestVersion || launch.Hash != buildManifestHash(launch) || compiled.Hash != launch.Hash {
		result.Status = "mismatch"
		result.Reasons = append(result.Reasons, "compiled manifest and launch dependencies differ or are invalid")
		return result
	}
	if receipt.Version == "" || executingDigest == "" {
		result.Reasons = append(result.Reasons, "launcher receipt or executing binary digest unavailable")
		return result
	}
	ownedReceipt := cloneJSON(receipt)
	result.Receipt = &ownedReceipt
	result.ExecutableHash = executingDigest
	if receipt.Version != BuildReceiptVersion || !evidenceHash.MatchString(executingDigest) || receipt.ManifestHash != compiled.Hash || receipt.ExecutableHash != executingDigest || receipt.LauncherHash != compiled.Sources["scripts/lab.mjs"] || receipt.Runtime != compiled.Runtime || !reflect.DeepEqual(receipt.Command, BuildCommand()) {
		result.Status = "mismatch"
		result.Reasons = append(result.Reasons, "launcher receipt does not match compiled manifest, executable, launcher or build recipe")
		return result
	}
	result.Status = "verified"
	return result
}

func ptrBuildManifest(manifest BuildManifest) *BuildManifest { return &manifest }

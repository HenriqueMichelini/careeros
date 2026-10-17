package evaluationlab

import "sync"

const ProcessResultVersion = "lab-process-result-v1"

type ProcessCapabilities struct {
	CoverageGaps       []string                    `json:"coverageGaps"`
	Selectors          []ProcessSelectorCapability `json:"selectors"`
	IsolationAvailable bool                        `json:"isolationAvailable"`
	RuntimeVersions    map[string]string           `json:"runtimeVersions"`
	UnavailableReasons []string                    `json:"unavailableReasons"`
}

type ProcessResult struct {
	TreeReady          bool              `json:"treeReady,omitempty"`
	NamespaceStopped   bool              `json:"namespaceStopped,omitempty"`
	Evidence           map[string]bool   `json:"evidence,omitempty"`
	Artifacts          []ProcessArtifact `json:"artifacts,omitempty"`
	Version            string            `json:"version"`
	Selector           string            `json:"selector"`
	Origin             string            `json:"origin"`
	Outcome            string            `json:"outcome"`
	ExitCode           *int              `json:"exitCode"`
	DurationMS         float64           `json:"durationMs"`
	RuntimeVersions    map[string]string `json:"runtimeVersions"`
	SourceHash         string            `json:"sourceHash"`
	SpawnAttempted     bool              `json:"spawnAttempted"`
	CleanupComplete    bool              `json:"cleanupComplete"`
	OutputText         string            `json:"outputText"`
	OutputHash         string            `json:"capturedOutputHash"`
	OutputTruncated    bool              `json:"outputTruncated"`
	UnavailableReasons []string          `json:"unavailableReasons"`
}

type ProcessArtifact struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Origin  string `json:"origin"`
	Runtime string `json:"runtime"`
	Hash    string `json:"hash"`
	Bytes   []byte `json:"bytes"`
}

// ProcessAdapter owns fixed reviewed inputs and private runtime isolation. It
// has no public command, path, environment, provider-host or credential input.
type ProcessAdapter struct {
	root          string
	sources       map[string]string
	tools         map[string]string
	capabilities  ProcessCapabilities
	mu            sync.Mutex
	cleanupFailed bool
}

func (p *ProcessAdapter) Capabilities() ProcessCapabilities { return cloneJSON(p.capabilities) }

type ProcessSelectorCapability struct {
	ID                 string   `json:"id"`
	RequiredTools      []string `json:"requiredTools"`
	Available          bool     `json:"available"`
	UnavailableReasons []string `json:"unavailableReasons"`
}

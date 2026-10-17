//go:build !linux

package evaluationlab

import (
	"context"
	"fmt"
)

func NewProcessAdapter(root string) (*ProcessAdapter, error) {
	return &ProcessAdapter{root: root, capabilities: ProcessCapabilities{RuntimeVersions: map[string]string{}, UnavailableReasons: []string{"Linux private namespace isolation required; no unsandboxed fallback"}}}, nil
}
func (p *ProcessAdapter) run(context.Context, string, int64, func(string) error) (ProcessResult, error) {
	return ProcessResult{Version: ProcessResultVersion, Outcome: "sandbox_unavailable", CleanupComplete: true}, fmt.Errorf("isolated processes unavailable")
}

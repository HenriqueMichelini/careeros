package semanticeval

import (
	"context"
	"fmt"
)

// Batch is a deliberate sequential evaluation, including optional settings
// interleaving or saved-response replay. Keys are never serialized or discovered.
type Batch struct {
	Cases          []Case
	Configurations []string
	Mode           string
	Budget         *Budget
	Saved          map[string]RunResult
	OpenAIKey      string `json:"-"`
	TypeSafeKey    string `json:"-"`
}

func ExecuteBatch(ctx context.Context, batch Batch, retain func(Case, string, RunResult, error) error) error {
	if retain == nil {
		return fmt.Errorf("result retention callback required")
	}
	if batch.Mode != "controlled" && batch.Mode != "live" && batch.Mode != "replay" {
		return fmt.Errorf("unknown evaluation mode")
	}
	if batch.Mode != "live" && (batch.OpenAIKey != "" || batch.TypeSafeKey != "") {
		return fmt.Errorf("offline/controlled batch refuses credentials")
	}
	configurations := batch.Configurations
	if len(configurations) == 0 {
		configurations = []string{""}
	}
	for _, id := range configurations {
		if id != "" {
			if _, err := configuration(id); err != nil {
				return err
			}
		}
	}
	if batch.Mode == "replay" {
		for _, c := range batch.Cases {
			saved, ok := batch.Saved[c.ID]
			if !ok || saved.Task != c.Task {
				return fmt.Errorf("missing or incompatible replay case")
			}
		}
	}
	for _, c := range batch.Cases {
		for _, id := range configurations {
			if err := ctx.Err(); err != nil {
				return err
			}
			var run RunResult
			var err error
			if batch.Mode == "replay" {
				run = batch.Saved[c.ID]
				run.Mode = "replay"
				run.Role = c.Role
				run, err = Assess(c, run)
			} else if id == "" {
				run, err = RunContext(ctx, c, batch.Mode, batch.OpenAIKey, batch.TypeSafeKey)
			} else {
				run, err = RunConfiguredContext(ctx, c, batch.Mode, batch.OpenAIKey, batch.TypeSafeKey, id, batch.Budget)
			}
			if retentionErr := retain(c, id, run, err); retentionErr != nil {
				return retentionErr
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

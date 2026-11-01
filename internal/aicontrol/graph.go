package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// checkGraph constrains reached stages to the fixed workflow. Existing handlers
// own semantic gate acceptance; provider HTTP success does not establish it.
func (s *Session) checkGraph(ctx context.Context, tx *sql.Tx, leg Leg) error {
	if (leg.Stage == "field_validation") != (leg.Provider == "typesafe") {
		return ErrDenied
	}
	observed := func(stage string, scope bool) bool {
		var n int
		query := `SELECT count(*) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3 AND stage=$4 AND state='observed'`
		args := []any{s.h.cfg.Namespace, s.id, s.attempt, stage}
		if scope {
			base := strings.TrimSuffix(strings.TrimSuffix(leg.Coordinate, "/original"), "/answer")
			query += ` AND (coordinate=$5 OR coordinate=$5||'/original' OR coordinate=$5||'/answer')`
			args = append(args, base)
		}
		return tx.QueryRowContext(ctx, query, args...).Scan(&n) == nil && n > 0
	}
	switch s.workflow {
	case "cv_generation":
		if leg.Stage == "generation" {
			return nil
		}
		if leg.Stage == "statement_support_assessment" && observed("generation", false) {
			return nil
		}
	case "profile_review":
		if leg.Stage == "section_review" {
			return nil
		}
		if leg.Stage == "section_support_check" && observed("section_review", false) {
			return nil
		}
	case "application_draft":
		if leg.Stage == "field_validation" {
			return nil
		}
		if leg.Stage == "generation" && observed("field_validation", false) {
			return nil
		}
		if (leg.Stage == "resume_support" || leg.Stage == "artifact_support") && observed("generation", false) {
			return nil
		}
	case "qualification_gaps":
		if leg.Stage == "field_validation" {
			return nil
		}
		if leg.Stage == "qualification_matching" && observed("field_validation", false) {
			return nil
		}
		if leg.Stage == "requirement_evidence" && observed("field_validation", false) {
			var root map[string]json.RawMessage
			_ = json.Unmarshal(s.rootInput, &root)
			if observed("qualification_matching", false) || len(root["jobContext"]) > 0 {
				return nil
			}
		}
	case "profile_ingestion":
		if leg.Stage == "field_validation" {
			return nil
		}
		if !observed("field_validation", true) {
			return ErrDenied
		}
		var input struct {
			Stage *struct {
				Version int    `json:"version"`
				Kind    string `json:"kind"`
			} `json:"stage"`
			Receipt string `json:"receipt"`
		}
		if len(leg.StageInput) > 0 && json.Unmarshal(leg.StageInput, &input) != nil {
			return ErrDenied
		}
		kind := "legacy"
		version := 0
		if input.Stage != nil {
			kind = input.Stage.Kind
			version = input.Stage.Version
		}
		if leg.Stage == "extraction" && (kind == "legacy" || kind == "extract" || kind == "clarify") {
			return nil
		}
		if leg.Stage == "reconciliation" {
			if kind == "reconcile" && (version == 1 || version == 2) {
				return nil
			}
			if (kind == "legacy" || kind == "clarify") && observed("extraction", true) {
				return nil
			}
		}
	}
	return ErrDenied
}

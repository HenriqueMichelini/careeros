package evaluationlab

import (
	"encoding/json"
	"fmt"
	"strings"
)

type cvSourceReference struct {
	ProfileID, ID string
	Revision      int
}
type cvSnapshotFact struct {
	ID, Section, EntryID, Field, Text string
	Reference, Owner                  cvSourceReference
}

// These are evidence joins, not a replacement for the public client validator.
// Missing stages remain unavailable; a different stage identity is rejected.
func joinCVSnapshot(observation json.RawMessage, language string) error {
	var receipt struct {
		Request struct {
			Facts   []cvSnapshotFact
			Density string
		}
		Document struct {
			ID       string
			Entities []struct{ ID, LegacyID string }
		}
		Units     []cvStatement
		Decisions struct {
			EditedText                                            map[string]string
			ConfirmedIDs, DiscardedSummaryIDs, RevertedWordingIDs []string
		}
		Snapshot, Reloaded json.RawMessage
	}
	if json.Unmarshal(observation, &receipt) != nil {
		return fmt.Errorf("invalid CV client receipt")
	}
	if len(receipt.Snapshot) == 0 || string(receipt.Snapshot) == "null" {
		if len(receipt.Reloaded) > 0 && string(receipt.Reloaded) != "null" {
			return fmt.Errorf("CV reload has no snapshot identity")
		}
		return nil
	}
	var saved struct {
		Version                                       int
		SourceProfileID, CVLanguage, Density, Summary string
		Sources                                       []cvSnapshotFact
		SummarySources                                []struct {
			Text      string
			SourceIDs []string
		}
		Provenance []struct {
			ID, Kind, Origin, Decision, Text string
			SourceIDs                        []string
			Original                         *cvStatement
		}
	}
	if json.Unmarshal(receipt.Snapshot, &saved) != nil || saved.Version != 3 || saved.SourceProfileID != receipt.Document.ID || saved.CVLanguage != language || saved.Density != receipt.Request.Density {
		return fmt.Errorf("CV snapshot request/profile identity mismatch")
	}
	facts := map[string]cvSnapshotFact{}
	for _, fact := range receipt.Request.Facts {
		facts[fact.ID] = fact
	}
	seenSources := map[string]bool{}
	for _, fact := range saved.Sources {
		original, ok := facts[fact.ID]
		entryMatches := fact.EntryID == original.EntryID
		for _, entity := range receipt.Document.Entities {
			if entity.ID == original.Owner.ID && entity.LegacyID == fact.EntryID {
				entryMatches = true
			}
		}
		// Request aliases and saved legacy IDs resolve to the same stable owner.
		fact.EntryID = original.EntryID
		if !ok || !entryMatches || seenSources[fact.ID] || !equalJSON(original, fact) {
			return fmt.Errorf("CV snapshot source identity mismatch")
		}
		seenSources[fact.ID] = true
	}
	units := map[string]cvStatement{}
	for _, unit := range receipt.Units {
		units[unit.ID] = unit
	}
	if len(saved.Provenance) != len(units) {
		return fmt.Errorf("CV snapshot statement population mismatch")
	}
	seen := map[string]bool{}
	activeSummary := []struct {
		Text      string
		SourceIDs []string
	}{}
	for _, record := range saved.Provenance {
		unit, ok := units[record.ID]
		if !ok || seen[record.ID] || !equalJSON(unit.SourceIDs, record.SourceIDs) {
			return fmt.Errorf("CV snapshot provenance identity mismatch")
		}
		seen[record.ID] = true
		originalUnit := unit
		originalUnit.ID = ""
		for _, id := range record.SourceIDs {
			if !seenSources[id] {
				return fmt.Errorf("CV snapshot cited source missing")
			}
		}
		switch record.Origin {
		case "generated":
			if record.Text != unit.Text {
				return fmt.Errorf("CV generated snapshot wording mismatch")
			}
			if record.Decision != "keep_generated" && record.Decision != "discard" && record.Decision != "revert_to_source" {
				return fmt.Errorf("unknown generated decision")
			}
		case "user_edited":
			if record.Decision != "edit" || receipt.Decisions.EditedText[record.ID] != record.Text || record.Original == nil || !equalJSON(*record.Original, originalUnit) {
				return fmt.Errorf("CV edited provenance original mismatch")
			}
		case "user_confirmed":
			if record.Decision != "confirm" || record.Text != unit.Text || !contains(receipt.Decisions.ConfirmedIDs, record.ID) || record.Original == nil || !equalJSON(*record.Original, originalUnit) {
				return fmt.Errorf("CV confirmed provenance original mismatch")
			}
		default:
			return fmt.Errorf("unknown CV snapshot provenance")
		}
		if record.Kind == "summary" && record.Decision != "discard" {
			activeSummary = append(activeSummary, struct {
				Text      string
				SourceIDs []string
			}{record.Text, record.SourceIDs})
		}
	}
	if !equalJSON(activeSummary, saved.SummarySources) {
		return fmt.Errorf("CV snapshot active summary mismatch")
	}
	texts := []string{}
	for _, summary := range activeSummary {
		texts = append(texts, summary.Text)
	}
	if saved.Summary != strings.Join(texts, " ") {
		return fmt.Errorf("CV snapshot rendered wording mismatch")
	}
	if len(receipt.Reloaded) > 0 && string(receipt.Reloaded) != "null" {
		var original, reloaded any
		if json.Unmarshal(receipt.Snapshot, &original) != nil || json.Unmarshal(receipt.Reloaded, &reloaded) != nil || !equalJSON(original, reloaded) {
			return fmt.Errorf("CV reload snapshot identity mismatch")
		}
	}
	return nil
}

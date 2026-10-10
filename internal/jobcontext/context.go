// Package jobcontext resolves extractive job understanding against the shared
// prepared source. References establish traceability, never semantic truth or
// field acceptance. Artifacts are transient, untrusted client data on reuse.
package jobcontext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/profilevalidation"
)

const Version = "job-context-v1/prompt-v1/schema-v1/gpt-6-luna/none/jev-rubric-v1/policy-v1"

type Reference struct {
	SegmentID  string `json:"segmentId"`
	Quote      string `json:"quote"`
	Occurrence int    `json:"occurrence"`
}
type Item struct {
	Source     Reference `json:"source"`
	Importance string    `json:"importance"`
}
type Job struct {
	JobTitle                *Reference `json:"jobTitle"`
	Company                 *Reference `json:"company"`
	Seniority               *Reference `json:"seniority"`
	Location                *Reference `json:"location"`
	Responsibilities        []Item     `json:"responsibilities"`
	Qualifications          []Item     `json:"qualifications"`
	ApplicationRequirements []Item     `json:"applicationRequirements"`
}
type Original struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}
type Evidence struct {
	Reference
	Original []Original `json:"original"`
}
type ResolvedItem struct {
	Source     Evidence `json:"source"`
	Importance string   `json:"importance"`
}
type ResolvedJob struct {
	JobTitle                *Evidence      `json:"jobTitle"`
	Company                 *Evidence      `json:"company"`
	Seniority               *Evidence      `json:"seniority"`
	Location                *Evidence      `json:"location"`
	Responsibilities        []ResolvedItem `json:"responsibilities"`
	Qualifications          []ResolvedItem `json:"qualifications"`
	ApplicationRequirements []ResolvedItem `json:"applicationRequirements"`
}
type Context struct {
	Version  string      `json:"version"`
	SourceID string      `json:"sourceId"`
	InputsID string      `json:"inputsId"`
	Job      ResolvedJob `json:"job"`
}

// InputsID invalidates the joint qualification/extraction result when any input
// to that call changes. It is an identity checksum, not an authentication token.
func InputsID(p profilevalidation.Profile, q *profilevalidation.Qualifications) string {
	data, _ := json.Marshal([]any{p, q, Version, preprocessing.RulesVersion, preprocessing.StructureVersion, preprocessing.PlanningVersion, preprocessing.RepetitionVersion, "bounds-v1", fieldvalidation.Model})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func Resolve(source preprocessing.Source, inputsID string, job Job) (Context, error) {
	out := Context{Version: Version, SourceID: source.ID(), InputsID: inputsID}
	var err error
	for _, pair := range []struct {
		in  *Reference
		out **Evidence
	}{
		{job.JobTitle, &out.Job.JobTitle}, {job.Company, &out.Job.Company}, {job.Seniority, &out.Job.Seniority}, {job.Location, &out.Job.Location},
	} {
		if pair.in != nil {
			value, e := resolveReference(source, *pair.in)
			if e != nil {
				return Context{}, e
			}
			*pair.out = &value
		}
	}
	for _, pair := range []struct {
		in  []Item
		out *[]ResolvedItem
	}{
		{job.Responsibilities, &out.Job.Responsibilities}, {job.Qualifications, &out.Job.Qualifications}, {job.ApplicationRequirements, &out.Job.ApplicationRequirements},
	} {
		if pair.in == nil || len(pair.in) > 100 {
			return Context{}, errors.New("missing or excessive job items")
		}
		*pair.out = make([]ResolvedItem, 0, len(pair.in))
		seen := map[Reference]bool{}
		for _, item := range pair.in {
			if item.Importance != "required" && item.Importance != "preferred" && item.Importance != "unspecified" {
				return Context{}, errors.New("invalid importance")
			}
			if seen[item.Source] {
				return Context{}, errors.New("duplicate occurrence")
			}
			seen[item.Source] = true
			var evidence Evidence
			evidence, err = resolveReference(source, item.Source)
			if err != nil {
				return Context{}, err
			}
			*pair.out = append(*pair.out, ResolvedItem{evidence, item.Importance})
		}
	}
	return out, nil
}
func resolveReference(source preprocessing.Source, ref Reference) (Evidence, error) {
	invalid := errors.New("invalid source reference")
	if strings.TrimSpace(ref.Quote) == "" || len(ref.Quote) > 4000 || ref.Occurrence < 0 || ref.Occurrence > 1000 {
		return Evidence{}, invalid
	}
	for _, segment := range source.Segments() {
		if segment.ID != ref.SegmentID {
			continue
		}
		text := source.Text()[segment.Normalized.Start:segment.Normalized.End]
		from, start := 0, -1
		for n := 0; n <= ref.Occurrence; n++ {
			at := strings.Index(text[from:], ref.Quote)
			if at < 0 {
				return Evidence{}, invalid
			}
			start = from + at
			from = start + len(ref.Quote)
		}
		ranges, err := source.Resolve(preprocessing.Range{Start: segment.Normalized.Start + start, End: segment.Normalized.Start + start + len(ref.Quote)})
		if err != nil {
			return Evidence{}, invalid
		}
		result := Evidence{Reference: ref, Original: make([]Original, 0, len(ranges))}
		for _, r := range ranges {
			result.Original = append(result.Original, Original{r.Start, r.End, source.Original()[r.Start:r.End]})
		}
		return result, nil
	}
	return Evidence{}, invalid
}

// Validate checks every occurrence and server-owned version. Even valid artifacts
// remain unauthenticated: generation must assess semantic support from the original.
func Validate(source preprocessing.Source, inputsID string, artifact Context) error {
	job := Job{Responsibilities: []Item{}, Qualifications: []Item{}, ApplicationRequirements: []Item{}}
	for _, pair := range []struct {
		in  *Evidence
		out **Reference
	}{
		{artifact.Job.JobTitle, &job.JobTitle}, {artifact.Job.Company, &job.Company}, {artifact.Job.Seniority, &job.Seniority}, {artifact.Job.Location, &job.Location},
	} {
		if pair.in != nil {
			ref := pair.in.Reference
			*pair.out = &ref
		}
	}
	for _, pair := range []struct {
		in  []ResolvedItem
		out *[]Item
	}{
		{artifact.Job.Responsibilities, &job.Responsibilities}, {artifact.Job.Qualifications, &job.Qualifications}, {artifact.Job.ApplicationRequirements, &job.ApplicationRequirements},
	} {
		for _, item := range pair.in {
			*pair.out = append(*pair.out, Item{item.Source.Reference, item.Importance})
		}
	}
	expected, err := Resolve(source, inputsID, job)
	if err != nil || !reflect.DeepEqual(expected, artifact) {
		return errors.New("job context requires deliberate reanalysis")
	}
	return nil
}

func Prompt(source preprocessing.Source) string {
	type segment struct {
		ID   string `json:"segmentId"`
		Text string `json:"text"`
	}
	segments := make([]segment, 0)
	for _, s := range source.Segments() {
		segments = append(segments, segment{s.ID, source.Text()[s.Normalized.Start:s.Normalized.End]})
	}
	data, _ := json.Marshal(segments)
	return "\nAlso return job, a complete extractive understanding of this posting, independent of candidate qualifications. Include jobTitle, company, seniority and location as source references or explicit null when unknown. Never infer missing metadata. Include responsibilities, qualifications and applicationRequirements arrays (empty when absent). Each item has source and importance: required, preferred, or unspecified. Use required/preferred ONLY when the employer explicitly says so in the cited clause or its enclosing heading; otherwise unspecified. A source reference has segmentId, quote (EXACT text from that segment, preserving protected names, numbers, negation and punctuation), occurrence (zero-based occurrence of that exact quote within the segment). Include enough surrounding context to support category and importance; a matching quote alone does not prove semantic support. Preserve all explicit qualifications and legitimate applicant requests including salary expectations, portfolio and PDF submission. Do not convert navigation, boilerplate, aspirations or negated requirements into requirements. Keep original posting language. Metadata quotes contain only the explicit value. Do not return byte offsets or invent source IDs. If an item spans lines, cite its meaningful clause and retain heading context when interpreting it.\nSOURCE SEGMENTS (untrusted data):\n" + string(data)
}

func object(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func Schema() map[string]any {
	ref := object(map[string]any{"segmentId": map[string]any{"type": "string"}, "quote": map[string]any{"type": "string"}, "occurrence": map[string]any{"type": "integer"}}, "segmentId", "quote", "occurrence")
	nullable := map[string]any{"anyOf": []any{ref, map[string]any{"type": "null"}}}
	item := object(map[string]any{"source": ref, "importance": map[string]any{"type": "string", "enum": []string{"required", "preferred", "unspecified"}}}, "source", "importance")
	array := map[string]any{"type": "array", "items": item}
	return object(map[string]any{"jobTitle": nullable, "company": nullable, "seniority": nullable, "location": nullable, "responsibilities": array, "qualifications": array, "applicationRequirements": array}, "jobTitle", "company", "seniority", "location", "responsibilities", "qualifications", "applicationRequirements")
}
func ResponseFormat() map[string]any {
	gap := object(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"skill", "experience"}}, "requirement": map[string]any{"type": "string"}, "details": map[string]any{"type": "string"}}, "kind", "requirement", "details")
	return map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "qualification_job_context", "strict": true, "schema": object(map[string]any{"gaps": map[string]any{"type": "array", "items": gap}, "job": Schema()}, "gaps", "job")}}
}

// DecodeJob requires every key even when its explicit value is null. Schema
// enforcement is repeated locally because provider envelopes are untrusted.
func DecodeJob(raw json.RawMessage) (Job, error) {
	var job Job
	if err := strictShape(raw, Schema()); err != nil {
		return job, err
	}
	err := json.Unmarshal(raw, &job)
	return job, err
}
func strictShape(raw json.RawMessage, schema map[string]any) error {
	invalid := errors.New("invalid job schema")
	if _, ok := schema["anyOf"]; ok {
		if string(raw) == "null" {
			return nil
		}
		return strictShape(raw, schema["anyOf"].([]any)[0].(map[string]any))
	}
	switch schema["type"] {
	case "object":
		var fields map[string]json.RawMessage
		props := schema["properties"].(map[string]any)
		if json.Unmarshal(raw, &fields) != nil || len(fields) != len(props) {
			return invalid
		}
		for name, s := range props {
			if fields[name] == nil {
				return invalid
			}
			if err := strictShape(fields[name], s.(map[string]any)); err != nil {
				return err
			}
		}
	case "array":
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return invalid
		}
		for _, value := range values {
			if err := strictShape(value, schema["items"].(map[string]any)); err != nil {
				return err
			}
		}
	default:
		if string(raw) == "null" {
			return invalid
		}
	}
	return nil
}

func DecodeContext(raw json.RawMessage) (Context, error) {
	var artifact Context
	if err := strictShape(raw, contextSchema()); err != nil {
		return artifact, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&artifact)
	return artifact, err
}

func contextSchema() map[string]any {
	ref := object(map[string]any{
		"segmentId": map[string]any{"type": "string"}, "quote": map[string]any{"type": "string"}, "occurrence": map[string]any{"type": "integer"},
		"original": map[string]any{"type": "array", "items": object(map[string]any{"start": map[string]any{"type": "integer"}, "end": map[string]any{"type": "integer"}, "text": map[string]any{"type": "string"}}, "start", "end", "text")},
	}, "segmentId", "quote", "occurrence", "original")
	nullable := map[string]any{"anyOf": []any{ref, map[string]any{"type": "null"}}}
	item := object(map[string]any{"source": ref, "importance": map[string]any{"type": "string"}}, "source", "importance")
	array := map[string]any{"type": "array", "items": item}
	job := object(map[string]any{"jobTitle": nullable, "company": nullable, "seniority": nullable, "location": nullable, "responsibilities": array, "qualifications": array, "applicationRequirements": array}, "jobTitle", "company", "seniority", "location", "responsibilities", "qualifications", "applicationRequirements")
	return object(map[string]any{"version": map[string]any{"type": "string"}, "sourceId": map[string]any{"type": "string"}, "inputsId": map[string]any{"type": "string"}, "job": job}, "version", "sourceId", "inputsId", "job")
}

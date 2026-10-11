package applicationdraft

import "professional-information-repo/internal/profiledocument"

// Each alternative binds the complete reference, so the provider cannot mix
// offsets, quotes or fact identities from different catalog entries. This
// constrains syntax only; support remains subject to the independent judgment
// and the existing server-side source and claim guards.
func artifactCitationSchemas(career []profiledocument.Reference, jobs []jobSupport) (map[string]any, map[string]any, bool) {
	// Three singleton enums per choice plus seven state/scope values stay below
	// the provider's 1000-value limit. Do not truncate or use unconstrained fallback.
	if len(career)+len(jobs) > 300 {
		return nil, nil, false
	}
	refs := make([]map[string]any, 0, len(career))
	for _, ref := range career {
		refs = append(refs, objectSchema(map[string]any{
			"profileId": map[string]any{"type": "string", "enum": []string{ref.ProfileID}},
			"id":        map[string]any{"type": "string", "enum": []string{ref.ID}},
			"revision":  map[string]any{"type": "integer", "enum": []int64{ref.Revision}},
		}, "profileId", "id", "revision"))
	}
	excerpts := make([]map[string]any, 0, len(jobs))
	for _, s := range jobs {
		excerpts = append(excerpts, objectSchema(map[string]any{
			"start": map[string]any{"type": "integer", "enum": []int{s.Start}},
			"end":   map[string]any{"type": "integer", "enum": []int{s.End}},
			"quote": map[string]any{"type": "string", "enum": []string{s.Quote}},
		}, "start", "end", "quote"))
	}
	emptyRef := objectSchema(map[string]any{"profileId": map[string]string{"type": "string"}, "id": map[string]string{"type": "string"}, "revision": map[string]string{"type": "integer"}}, "profileId", "id", "revision")
	emptyJob := objectSchema(map[string]any{"start": map[string]string{"type": "integer"}, "end": map[string]string{"type": "integer"}, "quote": map[string]string{"type": "string"}}, "start", "end", "quote")
	return citationArray(refs, emptyRef), citationArray(excerpts, emptyJob), true
}

func citationArray(choices []map[string]any, emptyItem map[string]any) map[string]any {
	if len(choices) == 0 {
		return map[string]any{"type": "array", "items": emptyItem, "maxItems": 0}
	}
	return map[string]any{"type": "array", "items": map[string]any{"anyOf": choices}, "maxItems": 16}
}

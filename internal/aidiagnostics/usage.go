package aidiagnostics

import "encoding/json"

// Usage reads numeric metadata independently from the output contract. Bad or
// absent metadata is unavailable and cannot invalidate otherwise valid prose.
func Usage(raw json.RawMessage, inputKey, outputKey string) (input, output, reasoning, cached *int64) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	input = token(fields[inputKey])
	output = token(fields[outputKey])
	var prompt, completion map[string]json.RawMessage
	_ = json.Unmarshal(fields["prompt_tokens_details"], &prompt)
	_ = json.Unmarshal(fields["completion_tokens_details"], &completion)
	cached = token(prompt["cached_tokens"])
	reasoning = token(completion["reasoning_tokens"])
	return
}
func token(raw json.RawMessage) *int64 {
	var n *int64
	if json.Unmarshal(raw, &n) != nil || n != nil && *n < 0 {
		return nil
	}
	return n
}

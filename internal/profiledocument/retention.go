package profiledocument

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// CleanupRetainedEvidence removes only excerpts with no current supporting
// reference. Raw nested values are never decoded through float64 or projected.
// The new canonical revision makes outstanding saved-Profile bindings stale.
func CleanupRetainedEvidence(raw []byte) ([]byte, bool, error) {
	doc, err := Decode(raw)
	if err != nil {
		return nil, false, err
	}
	facts := map[string]int64{}
	for _, f := range doc.Facts {
		facts[f.ID] = f.Revision
	}
	referenced, forbidden := map[string]bool{}, map[string]bool{}
	for _, l := range doc.Links {
		if l.Kind != "supports" {
			continue
		}
		if _, exists := facts[l.From.ID]; !exists {
			forbidden[l.To.ID] = true
		}
		if l.State == "active" && facts[l.From.ID] == l.From.Revision {
			referenced[l.To.ID] = true
		}
	}
	discarded := map[string]bool{}
	for _, e := range doc.Evidence {
		if !referenced[e.ID] || forbidden[e.ID] {
			discarded[e.ID] = true
		}
	}
	if len(discarded) == 0 {
		return append([]byte(nil), raw...), false, nil
	}
	if doc.Revision >= 9007199254740991 {
		return nil, false, fmt.Errorf("Profile revision exhausted")
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(raw, &root); err != nil {
		return nil, false, err
	}
	var evidence, links, factRows []json.RawMessage
	json.Unmarshal(root["evidence"], &evidence)
	json.Unmarshal(root["links"], &links)
	json.Unmarshal(root["facts"], &factRows)
	kept := make([]json.RawMessage, 0, len(evidence))
	for i, e := range doc.Evidence {
		if !discarded[e.ID] {
			kept = append(kept, evidence[i])
		}
	}
	affected := map[string]bool{}
	for i, l := range doc.Links {
		if l.Kind != "supports" || !discarded[l.To.ID] {
			continue
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(links[i], &fields)
		fields["state"] = json.RawMessage(`"invalidated"`)
		links[i], _ = marshalRawObject(fields)
		affected[l.From.ID] = true
	}
	for i, f := range doc.Facts {
		if f.Owner.ID != doc.ID && !(affected[f.ID] && f.Support == "supported") {
			continue
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(factRows[i], &fields)
		if f.Owner.ID == doc.ID {
			var owner map[string]json.RawMessage
			json.Unmarshal(fields["owner"], &owner)
			owner["revision"], _ = json.Marshal(doc.Revision + 1)
			fields["owner"], _ = marshalRawObject(owner)
		}
		if affected[f.ID] && f.Support == "supported" {
			fields["support"] = json.RawMessage(`"invalidated"`)
		}
		factRows[i], _ = marshalRawObject(fields)
	}
	root["revision"], _ = json.Marshal(doc.Revision + 1)
	root["evidence"] = marshalRawArray(kept)
	root["links"] = marshalRawArray(links)
	root["facts"] = marshalRawArray(factRows)
	next, err := marshalRawObject(root)
	if err != nil {
		return nil, false, err
	}
	if _, err = Decode(next); err != nil {
		return nil, false, err
	}
	return next, true, nil
}

// encoding/json deliberately compacts RawMessage on Marshal. These writers
// preserve already-validated untouched subtrees, including opaque value bytes.
func marshalRawObject(fields map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range keys {
		if !json.Valid(fields[k]) {
			return nil, fmt.Errorf("invalid raw Profile field")
		}
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(k)
		b.Write(key)
		b.WriteByte(':')
		b.Write(fields[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
func marshalRawArray(rows []json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, row := range rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(row)
	}
	b.WriteByte(']')
	return b.Bytes()
}

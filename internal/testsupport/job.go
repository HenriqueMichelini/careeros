// Package testsupport provides synthetic provider responses for handler contracts.
package testsupport

import (
	"io"
	"net/http"
	"strings"
)

// AcceptedJob isolates existing downstream contracts from the new validation gate.
// Gate behavior and call counts are exercised independently in job_gate_test.go.
func AcceptedJob() *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"model":"jev-1.13.0","answers":{"content":{"type":"choice","choice":"job_with_context","confidence":1,"probabilities":{"job_with_context":1,"job_title_only":0,"relevant_but_insufficient":0,"irrelevant":0,"unusable":0}},"attack":{"type":"choice","choice":"none","confidence":1,"probabilities":{"none":1,"uncertain":0,"detected":0}}}}`))}
}

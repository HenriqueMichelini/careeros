package releasecapture

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"professional-information-repo/internal/aidiagnostics"
)

type envelope struct {
	Stage            string   `json:"stage"`
	Outcome          string   `json:"outcome"`
	RequestBody      string   `json:"requestBody,omitempty"`
	RequestEncoding  string   `json:"requestEncoding,omitempty"`
	RequestSHA256    string   `json:"requestSHA256,omitempty"`
	RequestBytes     *int     `json:"requestBytes"`
	ResponseBody     string   `json:"responseBody,omitempty"`
	ResponseEncoding string   `json:"responseEncoding,omitempty"`
	ResponseSHA256   string   `json:"responseSHA256,omitempty"`
	ResponseBytes    *int     `json:"responseBytes"`
	HTTPStatus       *int     `json:"httpStatus"`
	DurationMS       *float64 `json:"durationMs"`
	Admission        string   `json:"admission,omitempty"`
	Endpoint         string   `json:"endpoint,omitempty"`
}

type transport struct{ base http.RoundTripper }

// Client preserves the supplied client's deadlines and networking. Capture state
// belongs to one request context, never to this shared client or transport.
func Client(client *http.Client) *http.Client {
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = transport{base: base}
	return &copy
}

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	c, _ := r.Context().Value(contextKey{}).(*collector)
	if c == nil {
		return t.base.RoundTrip(r)
	}
	if err := r.Context().Err(); err != nil {
		c.stopped = true
		return nil, err
	}
	stage := aidiagnostics.StageName(r.Context())
	index := len(c.Envelopes)
	if c.stopped || index >= len(c.approval.Stages) || c.approval.Stages[index] != stage {
		c.stopped = true
		return nil, errors.New("release capture admission denied")
	}
	e := envelope{Stage: stage}
	c.Envelopes = append(c.Envelopes, e)
	entry := &c.Envelopes[index]
	raw, err := io.ReadAll(io.LimitReader(r.Body, (256<<10)+1))
	_ = r.Body.Close()
	secrets := append(append([]string{}, c.secrets...), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if containsSecret(raw, secrets) {
		c.stopped = true
		entry.Outcome = "credential_content"
		return nil, errors.New("release capture credential content withheld")
	}
	if err != nil || len(raw) > 256<<10 || c.used+len(raw) > maxCaptureBytes {
		c.stopped = true
		entry.Outcome = "capture_unavailable"
		return nil, errors.New("release capture request unavailable")
	}
	if !c.allowedEnvelope(r, stage, raw) {
		c.stopped = true
		entry.Outcome = "admission_denied"
		return nil, errors.New("release capture envelope denied")
	}
	entry.Admission = "release-egress-v1"
	entry.Endpoint = r.URL.String()
	c.used += len(raw)
	entry.RequestBody, entry.RequestEncoding = encodedBody(raw)
	requestBytes := len(raw)
	entry.RequestSHA256, entry.RequestBytes = digest(raw), &requestBytes
	if !c.approval.Dispatch {
		entry.Outcome = "not_dispatched"
		c.stopped, c.inspectionOnly = true, true
		return nil, errors.New("release capture inspection only")
	}
	request := r.Clone(r.Context())
	request.Body = io.NopCloser(bytes.NewReader(raw))
	started := time.Now()
	response, err := t.base.RoundTrip(request)
	if err != nil || response == nil || response.Body == nil {
		entry.Outcome = "transport_failure"
		entry.DurationMS = elapsed(started)
		c.stopped = true
		if err == nil {
			err = errors.New("release capture response unavailable")
		}
		return nil, err
	}
	status := response.StatusCode
	entry.HTTPStatus = &status
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		// Authentication errors commonly echo masked keys; retain status only.
		_ = response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(nil))
		entry.Outcome, entry.DurationMS = "authentication_failure", elapsed(started)
		c.stopped = true
		return response, nil
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, int64(maxCaptureBytes-c.used+1)))
	_ = response.Body.Close()
	entry.DurationMS = elapsed(started)
	if containsSecret(raw, secrets) {
		c.stopped = true
		entry.Outcome = "credential_content"
		return nil, errors.New("release capture credential content withheld")
	}
	if err != nil || c.used+len(raw) > maxCaptureBytes {
		c.stopped = true
		entry.Outcome = "capture_unavailable"
		return nil, errors.New("release capture response unavailable")
	}
	c.used += len(raw)
	entry.ResponseBody, entry.ResponseEncoding = encodedBody(raw)
	responseBytes := len(raw)
	entry.ResponseSHA256, entry.ResponseBytes = digest(raw), &responseBytes
	entry.Outcome = "captured"
	response.Body = io.NopCloser(bytes.NewReader(raw))
	return response, nil
}

func elapsed(started time.Time) *float64 {
	duration := float64(time.Since(started)) / float64(time.Millisecond)
	return &duration
}

func containsSecret(raw []byte, secrets []string) bool {
	value := string(raw)
	for depth := 0; depth < 64; depth++ {
		if sensitiveString(value, secrets) {
			return true
		}
		decoded := decodeEscapes(value)
		if decoded == value {
			return false
		}
		value = decoded
	}
	// Excessive nested escapes cannot be inspected completely within this bound.
	return true
}

func sensitiveString(value string, secrets []string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			return true
		}
	}
	return false
}

// Scan escape tokens independently of JSON document validity, so truncated
// objects and embedded malformed strings cannot hide an otherwise complete key.
func decodeEscapes(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var decoded strings.Builder
	for i := 0; i < len(value); {
		if value[i] == '\\' && i+1 < len(value) {
			text, width := escapeToken(value[i:])
			if width > 0 {
				decoded.WriteString(text)
				i += width
				continue
			}
		}
		decoded.WriteByte(value[i])
		i++
	}
	return decoded.String()
}

func escapeToken(value string) (string, int) {
	if value[1] == 'u' && len(value) >= 6 {
		widths := []int{6}
		if len(value) >= 12 && value[6:8] == `\u` {
			widths = []int{12, 6}
		}
		for _, width := range widths {
			var text string
			if json.Unmarshal([]byte(`"`+value[:width]+`"`), &text) == nil {
				return text, width
			}
		}
	}
	escapes := map[byte]string{'"': "\"", '\\': `\`, '/': "/", 'b': "\b", 'f': "\f", 'n': "\n", 'r': "\r", 't': "\t"}
	if text, ok := escapes[value[1]]; ok {
		return text, 2
	}
	return "", 0
}

func encodedBody(raw []byte) (string, string) {
	if utf8.Valid(raw) {
		return string(raw), "utf8"
	}
	return base64.StdEncoding.EncodeToString(raw), "base64"
}

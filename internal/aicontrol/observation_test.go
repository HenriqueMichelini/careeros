package aicontrol

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestA13_AppendOnlyProviderMetadataAndMissingRate(t *testing.T) {
	for _, tc := range []struct {
		name, provider, body string
		estimate             *int64
		unsupported          bool
	}{
		{"openai", "openai", `{"id":"must-not-store-envelope-id","model":"gpt-6-luna","service_tier":"standard","usage":{"prompt_tokens":10,"completion_tokens":6,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":2}},"choices":[{"message":{"content":"CAREER_CONTENT_NEVER_LEDGER"}}]}`, pointer(16), false},
		{"missing_rate", "openai", `{"model":"gpt-6-luna","service_tier":"unknown","usage":{"prompt_tokens":10,"completion_tokens":6}}`, nil, false},
		{"unsupported", "openai", `{"model":"gpt-6-luna","service_tier":"standard","usage":{"prompt_tokens":10,"completion_tokens":6,"prompt_tokens_details":{"cache_write_tokens":3}}}`, nil, true},
		{"invalid", "openai", `{"model":"gpt-6-luna","service_tier":"standard","usage":{"prompt_tokens":"10","completion_tokens":6}}`, nil, true},
		{"typesafe", "typesafe", `{"model":"jev-1.13.0","service_tier":"standard","usage":{"input_tokens":10,"output_tokens":6},"answers":{"content":"CAREER_CONTENT_NEVER_LEDGER"}}`, pointer(16), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, nil)
			f.h.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
				f.calls.Add(1)
				return &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{"req_synthetic_bound"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			var s *Session
			var resp *http.Response
			var err error
			if tc.provider == "openai" {
				s = f.open(t, "metadata")
				resp, err = send(s)
			} else {
				s, err = f.h.Open(context.Background(), f.p, "metadata", "profile_ingestion", []byte(`{"input":"synthetic","profile":{}}`))
				if err != nil {
					t.Fatal(err)
				}
				raw := []byte(`{"model":"jev-1.13.0","state":{"field":"ingestion","submission":"synthetic"},"questions":{}}`)
				d, e := s.Reserve(context.Background(), Leg{Provider: "typesafe", Coordinate: "ingest/legacy/original", Stage: "field_validation", Number: 1, Body: raw})
				if e != nil {
					t.Fatal(e)
				}
				r, _ := http.NewRequest(http.MethodPost, "https://api.typesafe.ai/v1/systemone", strings.NewReader(string(raw)))
				resp, err = s.invoke(context.Background(), d, r, raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != tc.body {
				t.Fatal("observation changed provider bytes")
			}
			resp.Body.Close()
			resp.Body.Close()
			var count int
			var raw []byte
			if err = f.db.QueryRow(`SELECT count(*) FROM ai_fixture_observation`).Scan(&count); err != nil || count != 2 {
				t.Fatal("append observations", count, err)
			}
			if err = f.db.QueryRow(`SELECT data FROM ai_fixture_observation WHERE sequence=1`).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "CAREER_CONTENT") || strings.Contains(string(raw), "must-not-store-envelope") {
				t.Fatal("provider envelope/content retained")
			}
			var o ProviderObservation
			if json.Unmarshal(raw, &o) != nil {
				t.Fatal("invalid observation")
			}
			if o.Provider != tc.provider || o.RequestID != "req_synthetic_bound" || o.Unsupported != tc.unsupported {
				t.Fatalf("metadata %+v", o)
			}
			if tc.estimate == nil {
				if o.Estimate != nil {
					t.Fatal("unknown priced", o)
				}
				if tc.name == "missing_rate" && (o.RateID != "" || o.RateState != "unsupported") {
					t.Fatal("unsupported rate label not explicit", o)
				}
			} else if o.Estimate == nil || *o.Estimate != *tc.estimate {
				t.Fatal("estimate", o)
			}
			if f.scope(t).Counted != 60 {
				t.Fatal("observation automatically released maximum")
			}
		})
	}
}

type failingObservationReader struct {
	done bool
	err  error
}

func (r *failingObservationReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, `{"model":`), r.err
}
func (*failingObservationReader) Close() error { return nil }
func TestA13_ObservationPreservesProviderReadError(t *testing.T) {
	f := setup(t, nil)
	sentinel := errors.New("synthetic read sentinel")
	f.h.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &failingObservationReader{err: sentinel}}, nil
	})
	r, e := send(f.open(t, "read_error"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(r.Body)
	r.Body.Close()
	if !errors.Is(e, sentinel) || string(b) != `{"model":` {
		t.Fatal("read semantics changed", string(b), e)
	}
	if f.scope(t).Counted != 60 {
		t.Fatal("read error erased exposure")
	}
}

package qualificationgaps

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	draft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/testsupport"
	"strings"
	"testing"
)

func TestUnderstoodJobHasExactOriginalEvidence(t *testing.T) {
	posting := "Java developer. AWS required.\r\nSend salary expectations and portfolio as PDF."
	content := `{"gaps":[],"job":{"jobTitle":{"segmentId":"s1","quote":"Java developer","occurrence":0},"company":null,"seniority":null,"location":null,"responsibilities":[],"qualifications":[{"source":{"segmentId":"s1","quote":"AWS required.","occurrence":0},"importance":"required"}],"applicationRequirements":[{"source":{"segmentId":"s2","quote":"Send salary expectations and portfolio as PDF.","occurrence":0},"importance":"unspecified"}]}}`
	w := analyzeJob(t, posting, content, "stop", false)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var result struct {
		JobContext struct {
			SourceID string `json:"sourceId"`
			Job      struct {
				Company        any `json:"company"`
				Qualifications []struct {
					Source struct {
						Original []struct {
							Start, End int
							Text       string
						} `json:"original"`
					} `json:"source"`
					Importance string `json:"importance"`
				} `json:"qualifications"`
			} `json:"job"`
		} `json:"jobContext"`
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.JobContext.SourceID == "" || len(result.JobContext.Job.Qualifications) != 1 {
		t.Fatal(w.Body.String())
	}
	evidence := result.JobContext.Job.Qualifications[0]
	if evidence.Importance != "required" || len(evidence.Source.Original) != 1 || evidence.Source.Original[0].Start != 16 || evidence.Source.Original[0].End != 29 || evidence.Source.Original[0].Text != "AWS required." || result.JobContext.Job.Company != nil {
		t.Fatal(w.Body.String())
	}
}

func analyzeJob(t *testing.T, posting, content, finish string, refusal bool) *httptest.ResponseRecorder {
	t.Helper()
	input := sampleRequest()
	input.JobPosting = posting
	data, _ := json.Marshal(input)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	raw["understandJob"] = true
	data, _ = json.Marshal(raw)
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var payload struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&payload)
		parts := strings.Split(payload.Messages[0].Content, "SOURCE SEGMENTS (untrusted data):\n")
		var segments []struct {
			SegmentID string `json:"segmentId"`
		}
		if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &segments) != nil || len(segments) < 1 {
			t.Fatal("missing source segments")
		}

		for i, segment := range segments {
			content = strings.ReplaceAll(content, fmt.Sprintf(`"segmentId":"s%d"`, i+1), `"segmentId":"`+segment.SegmentID+`"`)
		}
		envelope, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]any{"content": content, "refusal": func() any {
			if refusal {
				return "refused"
			}
			return nil
		}()}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(envelope)))}, nil
	})}
	req := httptest.NewRequest("POST", "/api/qualification-gaps", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(w, req)
	return w

}

const emptyJob = `{"jobTitle":null,"company":null,"seniority":null,"location":null,"responsibilities":[],"qualifications":[],"applicationRequirements":[]}`

func TestJobUnderstandingRejectsMalformedAndIncompleteOutput(t *testing.T) {
	valid := `{"gaps":[],"job":` + emptyJob + `}`
	for _, tc := range []struct {
		name, content, finish string
		refusal               bool
	}{
		{"missing context", `{"gaps":[]}`, "stop", false},
		{"missing metadata", strings.Replace(valid, `"company":null,`, "", 1), "stop", false},
		{"missing quote", strings.Replace(valid, `"jobTitle":null`, `"jobTitle":{"segmentId":"s1","quote":"invented","occurrence":0}`, 1), "stop", false},
		{"protected value rewritten", strings.Replace(valid, `"jobTitle":null`, `"jobTitle":{"segmentId":"s1","quote":"Java developer. AWS optional.","occurrence":0}`, 1), "stop", false},
		{"bad occurrence", strings.Replace(valid, `"jobTitle":null`, `"jobTitle":{"segmentId":"s1","quote":"AWS","occurrence":2}`, 1), "stop", false},
		{"truncated", valid, "length", false}, {"refused", valid, "stop", true}, {"malformed", "not json", "stop", false},
		{"invalid importance", strings.Replace(valid, `"qualifications":[]`, `"qualifications":[{"source":{"segmentId":"s1","quote":"AWS required.","occurrence":0},"importance":"mandatory"}]`, 1), "stop", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := analyzeJob(t, "Java developer. AWS required.", tc.content, tc.finish, tc.refusal)
			if w.Code != 502 || !strings.Contains(w.Body.String(), "invalid_output") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestJobUnderstandingPreservesPortugueseUnicodeAndRepeatedOccurrences(t *testing.T) {
	posting := "Café  Java preferível. Café  Java preferível.\r\nEnvie pretensão salarial e portfólio em PDF."
	content := `{"gaps":[],"job":{"jobTitle":null,"company":null,"seniority":null,"location":null,"responsibilities":[],"qualifications":[{"source":{"segmentId":"s1","quote":"Café  Java preferível.","occurrence":0},"importance":"preferred"},{"source":{"segmentId":"s1","quote":"Café  Java preferível.","occurrence":1},"importance":"preferred"}],"applicationRequirements":[{"source":{"segmentId":"s2","quote":"Envie pretensão salarial e portfólio em PDF.","occurrence":0},"importance":"unspecified"}]}}`
	w := analyzeJob(t, posting, content, "stop", false)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var output struct {
		Context struct {
			Job struct {
				Qualifications []struct {
					Source struct {
						Original []struct {
							Start, End int
							Text       string
						}
					}
				}
			}
		} `json:"jobContext"`
	}
	json.Unmarshal(w.Body.Bytes(), &output)
	if len(output.Context.Job.Qualifications) != 2 {
		t.Fatal(w.Body.String())
	}
	first := output.Context.Job.Qualifications[0].Source.Original
	second := output.Context.Job.Qualifications[1].Source.Original
	if len(first) != 1 || len(second) != 1 || first[0].Start != 0 || second[0].Start != 25 || first[0].Text != "Café  Java preferível." || second[0].Text != first[0].Text {
		t.Fatal(w.Body.String())
	}
}

func TestReusingJobContextRequiresExactSourceVersionsAndWholeOriginalGate(t *testing.T) {
	posting := "Java developer. AWS required."
	analyzed := analyzeJob(t, posting, `{"gaps":[],"job":`+emptyJob+`}`, "stop", false)
	if analyzed.Code != 200 {
		t.Fatal(analyzed.Body.String())
	}
	for _, name := range []string{"valid", "changed original", "version", "inputs", "source", "malformed", "unknown field", "missing field", "altered profile", "forged original", "rejected gate"} {
		t.Run(name, func(t *testing.T) {
			input := sampleRequest()
			input.JobPosting = posting
			data, _ := json.Marshal(input)
			var raw map[string]any
			json.Unmarshal(data, &raw)
			delete(raw, "understandJob")
			raw["confirmedQualifications"] = []any{}
			var response map[string]any
			json.Unmarshal(analyzed.Body.Bytes(), &response)
			artifact := response["jobContext"].(map[string]any)
			raw["jobContext"] = artifact
			switch name {
			case "changed original":
				raw["jobPosting"] = posting + " "
			case "version":
				artifact["version"] = "old"
			case "inputs":
				artifact["inputsId"] = strings.Repeat("a", 64)
			case "source":
				artifact["sourceId"] = strings.Repeat("a", 64)
			case "malformed":
				raw["jobContext"] = nil
			case "unknown field":
				artifact["accepted"] = true
			case "missing field":
				delete(artifact["job"].(map[string]any), "company")
			case "altered profile":
				raw["repository"].(map[string]any)["skills"] = "Python"
			case "forged original":
				artifact["job"].(map[string]any)["jobTitle"] = map[string]any{"segmentId": "fake", "quote": "CEO", "occurrence": 0, "original": []any{}}
			}
			calls, gateCalls := 0, 0
			client := &http.Client{Transport: transportFuncDraft(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.typesafe.ai" {
					gateCalls++
					var payload struct{ State map[string]string }
					json.NewDecoder(r.Body).Decode(&payload)
					if payload.State["submission"] != raw["jobPosting"] {
						t.Fatal("gate did not receive whole exact original")
					}
					if name == "rejected gate" {
						accepted := testsupport.AcceptedJob()
						body, _ := io.ReadAll(accepted.Body)
						accepted.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(strings.ReplaceAll(string(body), `"choice":"none"`, `"choice":"detected"`), `"none":1,"uncertain":0,"detected":0`, `"none":0,"uncertain":0,"detected":1`)))
						return accepted, nil
					}
					return testsupport.AcceptedJob(), nil
				}
				calls++
				var payload struct{ Messages []struct{ Content string } }
				json.NewDecoder(r.Body).Decode(&payload)
				if !strings.Contains(payload.Messages[0].Content, "UNAUTHENTICATED SOURCE-BACKED JOB CONTEXT") || !strings.Contains(payload.Messages[0].Content, "Independently assess") {
					t.Fatal("client interpretation trusted")
				}
				content := `{"jobTitle":null,"company":null,"jobSummary":"AWS required.","resume":"Java","applicationAnswers":"I use Java.","coverLetter":{"greeting":"Dear team,","body":"I use Java.","closing":"Sincerely,"}}`
				envelope, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(envelope)))}, nil
			})}
			encoded, _ := json.Marshal(raw)
			req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(encoded)))
			req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
			req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
			w := httptest.NewRecorder()
			draft.NewHandlerWithClient(client).ServeHTTP(w, req)
			if name == "valid" {
				if w.Code != 200 || calls != 1 {
					t.Fatalf("%d %s", w.Code, w.Body.String())
				}
			} else if name == "rejected gate" {
				if calls != 0 || !strings.Contains(w.Body.String(), "reject_attack") {
					t.Fatalf("%d %s", w.Code, w.Body.String())
				}
			} else if w.Code != 502 || calls != 0 || !strings.Contains(w.Body.String(), "job_context_stale") {
				t.Fatalf("%d %s calls %d", w.Code, w.Body.String(), calls)
			}
			if gateCalls != 1 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("gate or privacy contract lost")
			}
		})
	}
}

type transportFuncDraft func(*http.Request) (*http.Response, error)

func (f transportFuncDraft) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFullNoisyPostingKeepsMetadataDutiesAndExplicitImportance(t *testing.T) {
	posting := "HOME | LOGIN\nRole: Backend engineer — Harbor Works\nLocation: Remote\nResponsibilities:\n- Build Java APIs.\nPreferred:\n- AWS experience.\nApply: Send salary expectations and portfolio as PDF."
	content := `{"gaps":[],"job":{"jobTitle":{"segmentId":"s2","quote":"Backend engineer","occurrence":0},"company":{"segmentId":"s2","quote":"Harbor Works","occurrence":0},"seniority":null,"location":{"segmentId":"s3","quote":"Remote","occurrence":0},"responsibilities":[{"source":{"segmentId":"s5","quote":"Build Java APIs.","occurrence":0},"importance":"unspecified"}],"qualifications":[{"source":{"segmentId":"s7","quote":"AWS experience.","occurrence":0},"importance":"preferred"}],"applicationRequirements":[{"source":{"segmentId":"s8","quote":"Send salary expectations and portfolio as PDF.","occurrence":0},"importance":"unspecified"}]}}`
	w := analyzeJob(t, posting, content, "stop", false)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, want := range []string{`"quote":"Backend engineer"`, `"quote":"Harbor Works"`, `"seniority":null`, `"quote":"Remote"`, `"quote":"Build Java APIs."`, `"importance":"preferred"`, `"quote":"Send salary expectations and portfolio as PDF."`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatal(w.Body.String())
		}
	}
	if strings.Contains(w.Body.String(), "HOME") {
		t.Fatal("navigation became a requirement")
	}
}

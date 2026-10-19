package profileingestion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func stagedApp(t *testing.T, counts *[3]int) app {
	t.Helper()
	return app{signingKey: []byte(strings.Repeat("s", 32)), client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > timeout {
			t.Fatal("unbounded provider")
		}
		if !strings.Contains(r.URL.Host, "openai.com") {
			counts[0]++
			return jevResponse("professional_fact", "none"), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), `"name":"profile_claims"`) {
			counts[1]++
			return completion(`{"claims":[{"id":"c1","source":"I use Java.","text":"Uses Java","targets":["skills"],"question":""}]}`), nil
		}
		counts[2]++
		return completion(`{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"add","value":"Java","finding":"addition"}]}`), nil
	})}}
}

func extractForStage(t *testing.T, a app, in request) request {
	t.Helper()
	in.Stage = &stageRequest{1, "extract"}
	w := send(t, a.handler(), in)
	var response struct {
		Receipt   string
		ExpiresAt int64
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Receipt == "" || response.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("extraction status=%d body=%s", w.Code, w.Body.String())
	}
	in.Stage = &stageRequest{1, "reconcile"}
	in.Receipt = response.Receipt
	return in
}

func TestStagesKeepOneOpenAICallPerRequestAndPreserveResult(t *testing.T) {
	var counts [3]int
	a := stagedApp(t, &counts)
	in := extractForStage(t, a, request{Input: "I use Java.", Profile: profile()})
	if counts != [3]int{1, 1, 0} {
		t.Fatal(counts)
	}
	w := send(t, a.handler(), in)
	var got result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Operations) != 1 || got.Operations[0].Value != "Java" || counts != [3]int{2, 1, 1} {
		t.Fatalf("status=%d calls=%v body=%s", w.Code, counts, w.Body.String())
	}
	// A deliberate comparison retry revalidates the original, never re-extracts.
	w = send(t, a.handler(), in)
	if w.Code != 200 || counts != [3]int{3, 1, 2} {
		t.Fatal(w.Code, counts)
	}
}

func TestInvalidReceiptsNeverDispatch(t *testing.T) {
	var counts [3]int
	a := stagedApp(t, &counts)
	valid := extractForStage(t, a, request{Input: "I use Java.", Profile: profile()})
	for _, kind := range []string{"signature", "expired", "version", "source", "profile", "portion", "clarification", "apply", "oversized", "missing-key"} {
		t.Run(kind, func(t *testing.T) {
			in := valid
			in.Profile = profile()
			candidate := a
			switch kind {
			case "signature":
				in.Receipt = "A" + in.Receipt[1:]
			case "source":
				in.Input += " More."
			case "profile":
				in.Profile.Skills += ", SQL"
			case "portion":
				in.Portion = &portionRequest{0, 800}
			case "clarification":
				in.Clarification = &clarificationRequest{Claim: claim{ID: "c1", Source: "I use Java.", Text: "Java"}, Answer: "At Acme"}
			case "apply":
				in.ApplyAnswer = &applyAnswerRequest{}
			case "oversized":
				in.Receipt = strings.Repeat("x", 180000)
			case "missing-key":
				candidate.signingKey = nil
			case "expired", "version":
				payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(in.Receipt, ".")[0])
				var receipt extractionReceipt
				_ = json.Unmarshal(payload, &receipt)
				if kind == "expired" {
					receipt.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
				} else {
					receipt.Version = "old"
				}
				payload = mustJSON(receipt)
				in.Receipt = base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(receiptMAC(a.signingKey, payload))
			}
			before := counts
			w := send(t, candidate.handler(), in)
			if w.Code < 400 || counts != before {
				t.Fatalf("status=%d calls=%v before=%v", w.Code, counts, before)
			}
		})
	}
}

type timeoutFailure struct{}

func (timeoutFailure) Error() string   { return "controlled timeout" }
func (timeoutFailure) Timeout() bool   { return true }
func (timeoutFailure) Temporary() bool { return true }

func TestProviderTimeoutErrorsAndCancellationAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		err        error
		cancel     bool
	}{
		{"client-timeout", "timeout", timeoutFailure{}, false},
		{"deadline", "timeout", context.DeadlineExceeded, false},
		{"transport", "outage", io.ErrUnexpectedEOF, false},
		{"cancel", "cancelled", context.Canceled, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			a := app{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, tc.err })}}
			_, code := a.sendProvider(ctx, "sk-inert", []byte(`{}`), "claims")
			if code != tc.want {
				t.Fatal(code)
			}
		})
	}
}

func TestStagedDeadlineStopsBeforeProviderWithoutWaiting(t *testing.T) {
	var counts [3]int
	a := stagedApp(t, &counts)
	in := request{Input: "I use Java.", Profile: profile(), Stage: &stageRequest{1, "extract"}}
	r := httptest.NewRequest("POST", "/api/profile/ingest", strings.NewReader(string(mustJSON(in))))
	r.Header.Set("X-OpenAI-Api-Key", "sk-inert")
	r.Header.Set("X-TypeSafe-Api-Key", "inert")
	ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(-time.Second))
	defer cancel()
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r.WithContext(ctx))
	if w.Code < 400 || counts != [3]int{} {
		t.Fatal(w.Code, counts)
	}
	_, code := a.sendProvider(ctx, "sk-inert", nil, "claims")
	if code != "timeout" || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal(code)
	}
}

func TestReceiptPayloadCapacityFailsWithoutTruncation(t *testing.T) {
	a := app{signingKey: []byte(strings.Repeat("s", 32))}
	_, _, code := a.issueReceipt(request{}, []claim{{Text: strings.Repeat("x", maxReceiptPayload)}}, 0, &reconciliationContext{})
	if code != "capacity" {
		t.Fatal(code)
	}
}

func TestClarificationReceiptRetainsAnswerOriginsAndGatesBothRequests(t *testing.T) {
	var counts [3]int
	a := stagedApp(t, &counts)
	in := extractForStage(t, a, request{Input: "I use Java.", Profile: profile(), Clarification: &clarificationRequest{Claim: claim{ID: "c1", Source: "I use Java.", Text: "Uses Java", Question: "Where?"}, Answer: "At Acme."}})
	if counts != [3]int{2, 1, 0} {
		t.Fatal(counts)
	}
	w := send(t, a.handler(), in)
	var got result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || counts != [3]int{4, 1, 1} || len(got.Claims) != 1 || len(got.Claims[0].SupportingSources) != 1 || got.Claims[0].SupportingSources[0].Origin != "clarification_answer" {
		t.Fatalf("status=%d counts=%v body=%s", w.Code, counts, w.Body.String())
	}
}

func TestReceiptBindingCanonicalizesObjectKeysButNotValuesOrArrays(t *testing.T) {
	a := request{Input: "Java", Document: json.RawMessage(`{"revision":9007199254740991,"facts":[{"value":"Java","id":"a"}]}`)}
	b := request{Input: "Java", Document: json.RawMessage(`{"facts":[{"id":"a","value":"Java"}],"revision":9007199254740991}`)}
	if requestBinding(a) != requestBinding(b) {
		t.Fatal("object key order changed binding")
	}
	b.Document = json.RawMessage(`{"facts":[{"id":"a","value":"Go"}],"revision":9007199254740991}`)
	if requestBinding(a) == requestBinding(b) {
		t.Fatal("changed value retained binding")
	}
}

type delayedStageBody struct{ ctx context.Context }

func (b delayedStageBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (delayedStageBody) Close() error               { return nil }

type cancelledStageBody struct{ cancel context.CancelFunc }

func (b cancelledStageBody) Read([]byte) (int, error) { b.cancel(); return 0, context.Canceled }
func (cancelledStageBody) Close() error               { return nil }

func TestCancelledProviderBodyIsNotInvalidOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := app{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: cancelledStageBody{cancel}}, nil
	})}}
	_, code := a.sendProvider(ctx, "sk-inert", []byte(`{}`), "claims")
	if code != "cancelled" {
		t.Fatal(code)
	}
}

func TestStagedSlowHeadersAndBodyRetainTimeoutWithoutComparison(t *testing.T) {
	for _, point := range []string{"headers", "body"} {
		t.Run(point, func(t *testing.T) {
			calls := 0
			a := app{signingKey: []byte(strings.Repeat("s", 32)), client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if !strings.Contains(r.URL.Host, "openai.com") {
					return jevResponse("professional_fact", "none"), nil
				}
				if point == "headers" {
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				return &http.Response{StatusCode: 200, Body: delayedStageBody{r.Context()}}, nil
			})}}
			in := request{Input: "I use Java.", Profile: profile(), Stage: &stageRequest{1, "extract"}}
			r := httptest.NewRequest("POST", "/api/profile/ingest", strings.NewReader(string(mustJSON(in))))
			r.Header.Set("X-OpenAI-Api-Key", "sk-inert")
			r.Header.Set("X-TypeSafe-Api-Key", "inert")
			ctx, cancel := context.WithTimeout(r.Context(), 550*time.Millisecond)
			defer cancel()
			w := httptest.NewRecorder()
			a.handler().ServeHTTP(w, r.WithContext(ctx))
			if w.Code != 504 || !strings.Contains(w.Body.String(), `"error":"timeout"`) || calls != 2 {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
		})
	}
}

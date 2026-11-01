package aicontrol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/auth"
)

const reservationCrashExit = 14
const invocationCrashExit = 15

// These modes extend the independently reconstructed test process. Every send
// still crosses the actual Session authority and an in-memory fake provider.
func recoveryChild(t *testing.T, s *Session, mode string) bool {
	t.Helper()
	switch mode {
	case "crash_reserved":
		s.h.SetFault("before_claim", func() error { os.Exit(reservationCrashExit); return nil })
		return false
	case "abandon":
		state, err := s.h.Status(context.Background(), s.p, s.ID())
		if err != nil || len(state.Dispatches) != 1 {
			t.Fatalf("reservation lookup: %v %+v", err, state)
		}
		childBarrier(t)
		if err = s.Abandon(context.Background(), state.Dispatches[0].ID); err != nil && !errors.Is(err, ErrDenied) {
			t.Fatal(err)
		}
		return true
	case "status":
		state, err := s.h.Status(context.Background(), s.p, s.ID())
		if err != nil {
			t.Fatal(err)
		}
		if err = json.NewEncoder(os.Stdout).Encode(state); err != nil {
			t.Fatal(err)
		}
		return true
	case "review":
		now, err := time.Parse(time.RFC3339Nano, os.Getenv("AI_SYNTHETIC_REVIEW_AT"))
		if err != nil {
			t.Fatal(err)
		}
		s.h.cfg.Clock = &fixtureClock{now: now, healthy: true}
		if err = s.h.SyntheticReviewer().ReviewConservativeClosure(context.Background(), s.p, os.Getenv("AI_SYNTHETIC_DISPATCH")); err != nil {
			t.Fatal(err)
		}
		return true
	case "expiry_reserve", "expiry_claim":
		var d Dispatch
		var err error
		if mode == "expiry_claim" {
			d, err = s.Reserve(context.Background(), Leg{Provider: "openai", Coordinate: "request", Stage: "generation", Number: 1, Body: fixtureBody})
			if err != nil {
				t.Fatal(err)
			}
		}
		childBarrier(t)
		var response *http.Response
		if mode == "expiry_claim" {
			response, err = s.invoke(context.Background(), d, req(), fixtureBody)
		} else {
			response, err = send(s)
		}
		closeResp(response)
		if !errors.Is(err, ErrDenied) && !errors.Is(err, auth.ErrAuthenticationRequired) {
			t.Fatalf("expired authority result: %v", err)
		}
		return true
	}
	childBarrier(t)
	return false
}

func childBarrier(t *testing.T) {
	t.Helper()
	if os.Getenv("AI_SYNTHETIC_BARRIER") != "1" {
		return
	}
	if _, err := io.WriteString(os.Stdout, "ready\n"); err != nil {
		t.Fatal(err)
	}
	var start [1]byte
	if _, err := io.ReadFull(os.Stdin, start[:]); err != nil {
		t.Fatal(err)
	}
}

type controlledChild struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *bufio.Reader
	stderr bytes.Buffer
	waited bool
}

func startControlledChild(t *testing.T, cmd *exec.Cmd) *controlledChild {
	t.Helper()
	p := &controlledChild{cmd: cmd}
	cmd.Env = append(cmd.Env, "AI_SYNTHETIC_BARRIER=1")
	var err error
	p.input, err = cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	p.output = bufio.NewReader(stdout)
	cmd.Stderr = &p.stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.input.Close()
		if !p.waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	if line, err := p.output.ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("child did not reach barrier: %q %v", line, err)
	}
	return p
}

func (p *controlledChild) release(t *testing.T) {
	t.Helper()
	if _, err := p.input.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	p.input.Close()
}

func (p *controlledChild) wait(t *testing.T) {
	t.Helper()
	out, readErr := io.ReadAll(p.output)
	err := p.cmd.Wait()
	p.waited = true
	if readErr != nil || err != nil {
		t.Fatalf("child: %v read: %v\n%s\n%s", err, readErr, out, p.stderr.String())
	}
}

func runChild(t *testing.T, cmd *exec.Cmd) []byte {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	return out
}

func crashChild(t *testing.T, cmd *exec.Cmd, code int) {
	t.Helper()
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != code {
		t.Fatalf("expected crash %d: %v\n%s", code, err, out)
	}
}

func (f *fixture) invocationCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM ai_test_invocations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestA07_ReservationCrashAndProcessAbandonClaim(t *testing.T) {
	for _, order := range []string{"abandon_first", "claim_first", "race"} {
		t.Run(order, func(t *testing.T) {
			f := setup(t, nil)
			crashChild(t, f.child("reserved_crash", "crash_reserved"), reservationCrashExit)
			s := f.open(t, "reserved_crash")
			before := f.status(t, s)
			if len(before.Dispatches) != 1 || before.Dispatches[0].State != "reserved" || before.Dispatches[0].IntentAt != nil || f.invocationCount(t) != 0 {
				t.Fatalf("crash was not between reservation and intent: %+v", before)
			}
			for _, scope := range []string{"global", "provider:openai", "account:" + f.p.AccountID} {
				got, err := f.h.ScopeStatus(context.Background(), f.p, scope)
				if err != nil || got.Counted != 60 || got.Occupied != 1 {
					t.Fatalf("reservation did not survive process loss: %v %+v", err, got)
				}
			}
			abandon, claim := f.child("reserved_crash", "abandon"), f.child("reserved_crash", "send")
			switch order {
			case "abandon_first":
				runChild(t, abandon)
				runChild(t, claim)
			case "claim_first":
				runChild(t, claim)
				runChild(t, abandon)
			case "race":
				a := startControlledChild(t, abandon)
				b := startControlledChild(t, claim)
				a.release(t)
				b.release(t)
				a.wait(t)
				b.wait(t)
			}
			state := f.status(t, s).Dispatches[0]
			wantCalls, wantCounted, wantSlots := 1, int64(60), 1
			if state.State == "aborted" {
				wantCalls, wantCounted, wantSlots = 0, 0, 0
				if state.IntentAt != nil || order == "claim_first" {
					t.Fatalf("abandoned a claimed intent: %+v", state)
				}
			} else if state.State != "observed" || state.IntentAt == nil || order == "abandon_first" {
				t.Fatalf("unexpected claim winner: %+v", state)
			}
			// Further independently restarted commands cannot undo the winner,
			// refund twice, or resend a dispatch whose intent is durable.
			runChild(t, f.child("reserved_crash", "abandon"))
			runChild(t, f.child("reserved_crash", "send"))
			after := f.status(t, s)
			if len(after.Dispatches) != 1 || after.Dispatches[0].ID != state.ID || after.Dispatches[0].State != state.State || after.Dispatches[0].Counted != wantCounted {
				t.Fatalf("restarted command changed the dispatch winner: %+v", after)
			}
			for _, scope := range []string{"global", "provider:openai", "account:" + f.p.AccountID} {
				got, err := f.h.ScopeStatus(context.Background(), f.p, scope)
				if err != nil || got.Counted != wantCounted || got.Occupied != wantSlots || f.invocationCount(t) != wantCalls {
					t.Fatalf("winner changed after restart: %v %+v calls=%d", err, got, f.invocationCount(t))
				}
			}
		})
	}
}

func TestA09_ProcessCrashAfterInvocationBeforeObservation(t *testing.T) {
	f := setup(t, nil)
	crashChild(t, f.child("observation_crash", "crash_observation"), invocationCrashExit)
	s := f.open(t, "observation_crash")
	state := f.status(t, s)
	if len(state.Dispatches) != 1 {
		t.Fatal(state)
	}
	d := state.Dispatches[0]
	var observations int
	if err := f.db.QueryRow(`SELECT count(*) FROM ai_fixture_observation`).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if d.State != "intent" || d.IntentAt == nil || d.DurableBound == nil || d.LocalFinished || observations != 0 || f.invocationCount(t) != 1 {
		t.Fatalf("crash missed the invoked-but-unobserved boundary: %+v observations=%d", d, observations)
	}
	runChild(t, f.child("observation_crash", "send"))
	after := f.status(t, s)
	if len(after.Dispatches) != 1 || after.Dispatches[0].ID != d.ID || after.Dispatches[0].Counted != 60 || after.Dispatches[0].Category != "unknown_maximum" || after.Dispatches[0].Remote != "possibly_active" || after.Dispatches[0].LocalFinished || after.Dispatches[0].IntentAt == nil || after.Dispatches[0].DurableBound == nil || !after.Dispatches[0].IntentAt.Equal(*d.IntentAt) || !after.Dispatches[0].DurableBound.Equal(*d.DurableBound) || after.CustomerReserved || after.CustomerEffect || f.invocationCount(t) != 1 {
		t.Fatalf("restart changed the unknown obligation: %+v", after)
	}
	if err := f.db.QueryRow(`SELECT count(*) FROM ai_fixture_observation`).Scan(&observations); err != nil || observations != 0 {
		t.Fatalf("restart manufactured an observation: %v count=%d", err, observations)
	}
	for _, scope := range []string{"global", "provider:openai", "account:" + f.p.AccountID} {
		got, err := f.h.ScopeStatus(context.Background(), f.p, scope)
		if err != nil || got.Counted != 60 || got.Occupied != 1 {
			t.Fatalf("restart released possible expense/capacity: %v %+v", err, got)
		}
	}
}

// This is a synthetic create-only HTTP seam over Open, not a production API.
// It loses a real TCP acknowledgment after commit, without fabricating a SQL
// error or performing inference while creating/recovering an Action.
func TestA36_ActualLostCreateHTTPResponse(t *testing.T) {
	f := setup(t, nil)
	var loseResponse atomic.Bool
	loseResponse.Store(true)
	committed := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key   string
			Input json.RawMessage
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid command", http.StatusBadRequest)
			return
		}
		s, err := f.h.Open(r.Context(), f.p, body.Key, "cv_generation", body.Input)
		if err != nil {
			http.Error(w, "denied", http.StatusConflict)
			return
		}
		if loseResponse.CompareAndSwap(true, false) {
			committed <- s.ID()
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(struct{ ID string }{s.ID()})
	}))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	command := `{"Key":"lost_http","Input":{"synthetic":"root"}}`
	response, err := client.Post(server.URL, "application/json", strings.NewReader(command))
	if err == nil {
		response.Body.Close()
		t.Fatal("lost TCP acknowledgment unexpectedly arrived")
	}
	var id string
	select {
	case id = <-committed:
	default:
		t.Fatal("response was lost before the Action committed")
	}
	if f.invocationCount(t) != 0 {
		t.Fatal("create sent provider work")
	}
	runChild(t, f.child("lost_http", "create"))
	response, err = client.Post(server.URL, "application/json", strings.NewReader(command))
	if err != nil {
		t.Fatal(err)
	}
	var acknowledgment struct{ ID string }
	err = json.NewDecoder(response.Body).Decode(&acknowledgment)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || acknowledgment.ID != id || f.invocationCount(t) != 0 {
		t.Fatalf("create recovery changed identity or sent: %v %+v", err, acknowledgment)
	}
	for _, invalid := range []string{`{"Key":"lost_http","Input":{"synthetic":"changed"}}`, `{"Key":"","Input":{"synthetic":"root"}}`} {
		response, err = client.Post(server.URL, "application/json", strings.NewReader(invalid))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusConflict || f.invocationCount(t) != 0 {
			t.Fatal("changed/missing creation key created or sent")
		}
	}
	var actions int
	if err = f.db.QueryRow(`SELECT count(*) FROM ai_fixture_action`).Scan(&actions); err != nil || actions != 1 {
		t.Fatalf("lost acknowledgment created another Action: %v count=%d", err, actions)
	}
	runChild(t, f.child("lost_http", "send"))
	runChild(t, f.child("lost_http", "send"))
	if f.invocationCount(t) != 1 {
		t.Fatal("acknowledged execution did not invoke exactly once")
	}
}

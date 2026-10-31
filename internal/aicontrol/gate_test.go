package aicontrol

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSyntheticGateInvalidatesDelayedPermitAcrossReopening(t *testing.T) {
	g := NewSyntheticGate()
	g.Reconcile("fixture", 1)
	old, err := g.capture("fixture")
	if err != nil {
		t.Fatal(err)
	}
	g.Close("fixture")
	g.Reconcile("fixture", 1)
	calls := 0
	resp, err := old.invoke(context.Background(), func(context.Context) (*http.Response, error) { calls++; return nil, nil })
	if resp != nil || !errors.Is(err, ErrDenied) || calls != 0 {
		t.Fatal("delayed old permit survived close/reopen")
	}
	fresh, err := g.capture("fixture")
	if err != nil {
		t.Fatal(err)
	}
	resp, err = fresh.invoke(context.Background(), func(context.Context) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("synthetic"))}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls != 1 {
		t.Fatal("fresh explicit opening did not invoke once")
	}
}

func TestSyntheticGateCancelsAndDrainsActiveCallbackWithoutReleasingResult(t *testing.T) {
	g := NewSyntheticGate()
	g.Reconcile("fixture", 1)
	lease, _ := g.capture("fixture")
	started := make(chan struct{})
	canceled := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		resp, err := lease.invoke(context.Background(), func(ctx context.Context) (*http.Response, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-finish
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("PRIVATE_RESULT"))}, nil
		})
		if resp != nil {
			resp.Body.Close()
		}
		done <- err
	}()
	<-started
	g.Close("fixture")
	<-canceled
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := g.Drain(ctx, "fixture"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active callback falsely drained: %v", err)
	}
	g.Reconcile("fixture", 2)
	if g.permits("fixture", 2) {
		t.Fatal("reopened without local drain")
	}
	close(finish)
	if err := <-done; !errors.Is(err, ErrDenied) {
		t.Fatalf("closed gateway released response: %v", err)
	}
	if err := g.Drain(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	g.Reconcile("fixture", 2)
	if !g.permits("fixture", 2) || lease.allowed() == nil {
		t.Fatal("new opening revived old lease")
	}
}

type gateBlockingBody struct{ started, finish chan struct{} }

func (b gateBlockingBody) Read(p []byte) (int, error) {
	close(b.started)
	<-b.finish
	return copy(p, "PRIVATE_RESULT"), io.EOF
}
func (gateBlockingBody) Close() error { return nil }

func TestSyntheticGateWithholdsBodyIfCloseWinsBlockedRead(t *testing.T) {
	g := NewSyntheticGate()
	g.Reconcile("fixture", 1)
	lease, _ := g.capture("fixture")
	body := gateBlockingBody{make(chan struct{}), make(chan struct{})}
	resp, err := lease.invoke(context.Background(), func(context.Context) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil })
	if err != nil {
		t.Fatal(err)
	}
	type readResult struct {
		n   int
		err error
	}
	done := make(chan readResult, 1)
	go func() { buffer := make([]byte, 32); n, err := resp.Body.Read(buffer); done <- readResult{n, err} }()
	<-body.started
	g.Close("fixture")
	close(body.finish)
	result := <-done
	if result.n != 0 || !errors.Is(result.err, ErrDenied) {
		t.Fatalf("closed body released bytes: %+v", result)
	}
	if err = g.Drain(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
}

package semanticeval

import (
	"context"
	"io"
)

type DispatchPhase string

const (
	DispatchStarted  DispatchPhase = "started"
	DispatchFinished DispatchPhase = "finished"
)

type DispatchObservation struct {
	Phase DispatchPhase
	Call  Call
	Err   error
}

// ExecutionPolicy is supplied by the local runner, never imported report data.
// The controlled-body seam permits bounded transport/read failure evaluation;
// it is never applied to live transport and has no external fallback.
type ExecutionPolicy struct {
	BeforeWorkflow  func(context.Context, Case) error
	ObserveWorkflow func(Case, RunResult)
	BeforeDispatch  func(context.Context) error
	NewDispatchID   func() string
	ObserveDispatch func(DispatchObservation)
	ControlledBody  func(context.Context, io.ReadCloser) io.ReadCloser
}
type executionPolicyKey struct{}

type requestObservationKey struct{}

func WithRequestObservation(ctx context.Context, observe func(string, int)) context.Context {
	return context.WithValue(ctx, requestObservationKey{}, observe)
}
func observeRequest(ctx context.Context, provider string, size int) {
	if observe, ok := ctx.Value(requestObservationKey{}).(func(string, int)); ok {
		observe(provider, size)
	}
}

func WithExecutionPolicy(ctx context.Context, policy ExecutionPolicy) context.Context {
	return context.WithValue(ctx, executionPolicyKey{}, policy)
}
func executionPolicy(ctx context.Context) ExecutionPolicy {
	policy, _ := ctx.Value(executionPolicyKey{}).(ExecutionPolicy)
	return policy
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

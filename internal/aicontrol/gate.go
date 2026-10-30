package aicontrol

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// SyntheticGate is independent in-memory fixture authority, never a hosted
// recovery implementation. Close fences new handoffs and cancels active local
// callbacks; Drain separately proves those fixture callbacks/bodies exited. No
// operation here releases provider expense or possible-remote capacity.
type SyntheticGate struct {
	mu         sync.Mutex
	namespaces map[string]*gateNamespace
	changed    chan struct{}
}

type gateNamespace struct {
	epoch   int64
	version uint64
	open    bool
	retired bool
	active  map[*gateCall]struct{}
}
type gateCall struct{ cancel context.CancelFunc }
type gateLease struct {
	gate      *SyntheticGate
	namespace string
	epoch     int64
	version   uint64
}

func NewSyntheticGate() *SyntheticGate {
	return &SyntheticGate{namespaces: map[string]*gateNamespace{}, changed: make(chan struct{})}
}
func (g *SyntheticGate) notifyLocked() { close(g.changed); g.changed = make(chan struct{}) }
func (g *SyntheticGate) stateLocked(namespace string) *gateNamespace {
	n := g.namespaces[namespace]
	if n == nil {
		n = &gateNamespace{active: map[*gateCall]struct{}{}}
		g.namespaces[namespace] = n
	}
	return n
}

// Reconcile represents explicit test-owned external reconciliation. It is not
// evidence that restoring or incrementing a PostgreSQL row repaired anything.
// Every opening invalidates old leases, including an opening at the same epoch.
func (g *SyntheticGate) Reconcile(namespace string, epoch int64) {
	if g == nil || !tokenLabel.MatchString(namespace) || epoch <= 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	n := g.stateLocked(namespace)
	// An old callback/body must be drained before this local gateway reopens.
	if n.retired || len(n.active) != 0 {
		return
	}
	n.version++
	n.epoch = epoch
	n.open = true
	g.notifyLocked()
}

// Retire is irreversible for this independent fixture authority. Restoring a
// database row, changing a namespace epoch, or opening the same endpoint cannot
// revive an old lease or a namespace whose capability horizon was retired.
func (g *SyntheticGate) Retire(namespace string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	n := g.stateLocked(namespace)
	n.retired = true
	n.open = false
	n.version++
	for active := range n.active {
		active.cancel()
	}
	g.notifyLocked()
}
func (g *SyntheticGate) Close(namespace string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	n := g.stateLocked(namespace)
	n.open = false
	n.version++
	for active := range n.active {
		active.cancel()
	}
	g.notifyLocked()
}
func (g *SyntheticGate) permits(namespace string, epoch int64) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	n := g.namespaces[namespace]
	return n != nil && n.open && n.epoch == epoch
}
func (g *SyntheticGate) capture(namespace string) (gateLease, error) {
	if g == nil {
		return gateLease{}, ErrDenied
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	n := g.namespaces[namespace]
	if n == nil || !n.open {
		return gateLease{}, ErrDenied
	}
	return gateLease{g, namespace, n.epoch, n.version}, nil
}
func (l gateLease) validLocked() bool {
	n := l.gate.namespaces[l.namespace]
	return n != nil && n.open && n.epoch == l.epoch && n.version == l.version
}
func (l gateLease) allowed() error {
	if l.gate == nil {
		return ErrDenied
	}
	l.gate.mu.Lock()
	defer l.gate.mu.Unlock()
	if !l.validLocked() {
		return ErrDenied
	}
	return nil
}

// Drain is a bounded observation of fixture callbacks and response bodies, not
// remote termination proof. Calling it with a canceled context cannot open a gate.
func (g *SyntheticGate) Drain(ctx context.Context, namespace string) error {
	if g == nil {
		return ErrDenied
	}
	for {
		g.mu.Lock()
		n := g.namespaces[namespace]
		if n == nil || len(n.active) == 0 {
			g.mu.Unlock()
			return nil
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (l gateLease) invoke(ctx context.Context, fn func(context.Context) (*http.Response, error)) (*http.Response, error) {
	if l.gate == nil || fn == nil {
		return nil, ErrDenied
	}
	g := l.gate
	g.mu.Lock()
	if !l.validLocked() || ctx.Err() != nil {
		g.mu.Unlock()
		return nil, ErrDenied
	}
	callCtx, cancel := context.WithCancel(ctx)
	call := &gateCall{cancel: cancel}
	g.namespaces[l.namespace].active[call] = struct{}{}
	g.notifyLocked()
	g.mu.Unlock()
	var once sync.Once
	finish := func() {
		once.Do(func() {
			cancel()
			g.mu.Lock()
			delete(g.namespaces[l.namespace].active, call)
			g.notifyLocked()
			g.mu.Unlock()
		})
	}
	resp, err := fn(callCtx)
	if l.allowed() != nil || callCtx.Err() != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		finish()
		return nil, ErrDenied
	}
	if err != nil || resp == nil {
		finish()
		return resp, err
	}
	if resp.Body == nil {
		finish()
		return resp, nil
	}
	resp.Body = &gateBody{source: resp.Body, lease: l, ctx: callCtx, finish: finish}
	return resp, nil
}

type gateBody struct {
	source io.ReadCloser
	lease  gateLease
	ctx    context.Context
	finish func()
}

func (b *gateBody) Read(p []byte) (int, error) {
	if b.lease.allowed() != nil || b.ctx.Err() != nil {
		b.Close()
		return 0, ErrDenied
	}
	n, err := b.source.Read(p)
	// Bytes are withheld if closure wins while the underlying read is blocked.
	if b.lease.allowed() != nil || b.ctx.Err() != nil {
		b.Close()
		return 0, ErrDenied
	}
	if err != nil {
		b.finish()
	}
	return n, err
}
func (b *gateBody) Close() error { err := b.source.Close(); b.finish(); return err }

// ResultAllowed is also checked by the protected HTTP result handoff. A body
// parsed before Close cannot later release a protected result through an old lease.
func (s *Session) ResultAllowed(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrDenied
	}
	return s.gate.allowed()
}

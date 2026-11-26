package accountstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
	"sync/atomic"
	"time"
)

// HoldControlDispatch is the explicit local fake-handoff lease. It pins a
// shared PostgreSQL session lock, not an open transaction, until the callback
// returns. A winning exclusive operator stop cannot pass an old lease. This
// does not assert cancellation or physical exclusion of any live provider.
type ControlDispatchLease struct {
	connection *sql.Conn
	once       sync.Once
	closed     atomic.Bool
}

func HoldControlDispatch(ctx context.Context, db *sql.DB) (*ControlDispatchLease, error) {
	connection, err := db.Conn(ctx)
	if err != nil {
		return nil, unavailable(err)
	}
	discard := func() {
		_ = connection.Raw(func(any) error { return driver.ErrBadConn })
		_ = connection.Close()
	}
	if _, err = connection.ExecContext(ctx, "SELECT pg_advisory_lock_shared($1)", controlFence); err != nil {
		// A canceled acquisition has an ambiguous server-side outcome. Never
		// return a potentially locked session to the runtime connection pool.
		discard()
		return nil, unavailable(err)
	}
	return &ControlDispatchLease{connection: connection}, nil
}

// Begin validates on the same pinned backend that owns the session lease.
// Acquiring another shared lock on another pool connection behind a queued
// exclusive stop would create an application-level lock cycle.
func (l *ControlDispatchLease) Begin(ctx context.Context) (*sql.Tx, error) {
	if l == nil || l.connection == nil || l.closed.Load() {
		return nil, unavailable(sql.ErrConnDone)
	}
	tx, err := l.connection.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	return tx, nil
}

func (l *ControlDispatchLease) Close() {
	if l == nil || l.connection == nil {
		return
	}
	l.once.Do(func() {
		l.closed.Store(true)
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var released bool
		if err := l.connection.QueryRowContext(unlockCtx, "SELECT pg_advisory_unlock_shared($1)", controlFence).Scan(&released); err != nil || !released {
			_ = l.connection.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = l.connection.Close()
	})
}

// Discard ends an ambiguous leased operation without returning its backend to
// the pool. Disconnecting also releases that backend's advisory locks.
func (l *ControlDispatchLease) Discard() {
	if l == nil || l.connection == nil {
		return
	}
	l.once.Do(func() {
		l.closed.Store(true)
		_ = l.connection.Raw(func(any) error { return driver.ErrBadConn })
		_ = l.connection.Close()
	})
}

package aicontrol

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestA05_AbsentAuthorityOrFiniteEnvelopeFailsClosed(t *testing.T) {
	f := setup(t, nil)
	for _, tc := range []struct {
		name   string
		mutate func(*SyntheticConfig)
	}{
		{"clock", func(c *SyntheticConfig) { c.Clock = nil }},
		{"gate", func(c *SyntheticConfig) { c.Gate = nil }},
		{"binding_key", func(c *SyntheticConfig) { c.BindingKey = [32]byte{} }},
		{"money_overflow", func(c *SyntheticConfig) { c.Limit = math.MaxInt64 }},
		{"capacity", func(c *SyntheticConfig) { c.Capacity = 0 }},
		{"dispatches", func(c *SyntheticConfig) { c.MaxDispatches = 0 }},
		{"stages", func(c *SyntheticConfig) { c.MaxStages = 0 }},
		{"attempts", func(c *SyntheticConfig) { c.MaxAttempts = 0 }},
		{"input", func(c *SyntheticConfig) { c.MaxInputBytes = 0 }},
		{"output", func(c *SyntheticConfig) { c.MaxOutputTokens = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := f.cfg
			tc.mutate(&cfg)
			h, e := NewSynthetic(context.Background(), f.dsn, cfg)
			if e == nil {
				h.Close()
				t.Fatal("missing/invalid authority accepted")
			}
		})
	}
	for _, authority := range []string{"policy", "store"} {
		t.Run(authority, func(t *testing.T) {
			f := setup(t, nil)
			s := f.open(t, "missing_authority")
			if authority == "policy" {
				if _, e := f.db.Exec(`DELETE FROM ai_fixture_policy`); e != nil {
					t.Fatal(e)
				}
			} else {
				f.h.db.Close()
			}
			r, e := send(s)
			closeResp(r)
			if e == nil || f.calls.Load() != 0 {
				t.Fatal("missing authority sent", e, f.calls.Load())
			}
			var count int
			if e = f.db.QueryRow(`SELECT count(*) FROM ai_fixture_dispatch`).Scan(&count); e != nil || count != 0 {
				t.Fatal("missing authority partially reserved", count, e)
			}
		})
	}
}

func TestA05_MissingOrRevokedGrantCannotMintRights(t *testing.T) {
	f := setup(t, nil)
	if _, e := f.db.Exec(`DELETE FROM ai_fixture_grant`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.h.Open(context.Background(), f.p, "missing_grant", "cv_generation", []byte(`{"synthetic":"root"}`)); e == nil {
		t.Fatal("missing grant admitted")
	}
	if e := f.h.SyntheticReviewer().SeedGrant(context.Background(), f.p, time.Now().Add(time.Hour), 3); e != nil {
		t.Fatal(e)
	}
	s := f.openDraft(t, "revocation")
	if e := f.h.SyntheticReviewer().SetGrantState(context.Background(), f.p, time.Now().Add(time.Hour), true); e != nil {
		t.Fatal(e)
	}
	if r, e := sendDraftGate(s); e == nil {
		closeResp(r)
		t.Fatal("revoked grant sent")
	}
	if e := s.ReserveCustomer(context.Background()); e == nil {
		t.Fatal("revoked grant reserved customer unit")
	}
	if _, e := s.Retry(context.Background()); e == nil {
		t.Fatal("revoked grant retried")
	}
	if e := f.h.SyntheticReviewer().SeedGrant(context.Background(), f.p, time.Now().Add(time.Hour), 999); e != nil {
		t.Fatal(e)
	}
	var revoked bool
	var units int64
	if e := f.db.QueryRow(`SELECT revoked,units FROM ai_fixture_grant`).Scan(&revoked, &units); e != nil || !revoked || units != 3 {
		t.Fatal("reseed revived rights", revoked, units, e)
	}
	if f.calls.Load() != 0 {
		t.Fatal("negative invoked provider")
	}
}

func TestA35_DelayedIntentCannotOutliveSendAuthority(t *testing.T) {
	for _, authority := range []string{"session", "grant", "policy"} {
		t.Run(authority, func(t *testing.T) {
			f := setup(t, nil)
			if authority == "policy" {
				if _, e := f.db.Exec(`UPDATE ai_fixture_policy SET expires_at=clock_timestamp()+interval '250 milliseconds'`); e != nil {
					t.Fatal(e)
				}
			}
			s := f.open(t, "handoff_expiry")
			var e error
			switch authority {
			case "session":
				_, e = f.db.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '250 milliseconds' WHERE id=$1`, f.p.SessionID)
			case "grant":
				_, e = f.db.Exec(`UPDATE ai_fixture_grant SET expires_at=clock_timestamp()+interval '250 milliseconds'`)
			case "policy":
				// Original frozen policy simply expires while handoff is delayed.
			}
			if e != nil {
				t.Fatal(e)
			}
			f.h.SetFault("before_invoke", func() error { time.Sleep(300 * time.Millisecond); return nil })
			r, e := send(s)
			closeResp(r)
			if e == nil || f.calls.Load() != 0 {
				t.Fatal("expired delayed intent sent", e, f.calls.Load())
			}
			var state string
			var amount int64
			var slots int
			if e = f.db.QueryRow(`SELECT d.state,s.counted,s.occupied FROM ai_fixture_dispatch d CROSS JOIN ai_fixture_scope s WHERE s.scope='global'`).Scan(&state, &amount, &slots); e != nil {
				t.Fatal(e)
			}
			if state != "intent" || amount != 60 || slots != 1 {
				t.Fatal("denial erased intent obligation", state, amount, slots)
			}
		})
	}
}

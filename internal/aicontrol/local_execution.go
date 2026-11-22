package aicontrol

import (
	"context"
	"sync"
	"time"
)

// BeginLocalExecution records a code-owned invocation before entering a managed
// transient handler. Customer/content terminality cannot prove this invocation
// returned. Missing return evidence (including a process crash) stays held.
// The returned capability can retire only this invocation; it cannot publish a
// result, finish the Action, or reconcile any remote-call or financial hold.
func (s *Session) BeginLocalExecution(ctx context.Context) (func(), error) {
	if !s.h.cfg.Qualification || s.workflow == "application_draft" {
		return nil, ErrDenied
	}
	tx, err := s.h.begin(ctx, s.p)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.stageAction(ctx, tx); err != nil {
		return nil, err
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return nil, err
	}
	witness := opaque()
	if witness == "" {
		return nil, ErrUnavailable
	}
	coordinate := "local/" + witness
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) VALUES($1,$2,$3,'local_execution',$4,$5)`, s.h.cfg.Namespace, s.id, coordinate, s.attempt, s.root); err != nil {
		return nil, ErrUnavailable
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrUnavailable
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			// A canceled request or revoked account must not prevent truthful
			// local return bookkeeping. This narrow code capability uses the
			// existing reviewer connection; no runtime SQL grant is expanded.
			finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = s.h.reviewerDB.ExecContext(finishCtx, `UPDATE ai_fixture_stage SET receipt_id='returned' WHERE namespace=$1 AND action=$2 AND coordinate=$3 AND stage='local_execution' AND attempt=$4 AND binding=$5 AND receipt_id=''`, s.h.cfg.Namespace, s.id, coordinate, s.attempt, s.root)
		})
	}, nil
}

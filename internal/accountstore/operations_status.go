package accountstore

import (
	"context"
	"errors"
	"time"

	"professional-information-repo/internal/auth"
)

type OperationsPublicStatus struct {
	Impact       string     `json:"impact"`
	Status       string     `json:"status"`
	NextUpdateAt *time.Time `json:"nextUpdateAt,omitempty"`
	PublishedAt  time.Time  `json:"publishedAt"`
	Limitation   string     `json:"limitation,omitempty"`
}

// LocalOperationsStatus owns only the public stream. Opening it never touches
// the private incident stream or acquires a managed dispatch lease.
type LocalOperationsStatus struct{ journal *OperationsJournal }

func OpenLocalOperationsStatus(cfg OperationsJournalConfig) (*LocalOperationsStatus, error) {
	journal, err := openOperationsJournal(cfg, "status")
	if err != nil {
		return nil, err
	}
	return &LocalOperationsStatus{journal: journal}, nil
}

func (s *LocalOperationsStatus) PublishOutage(ctx context.Context, impact, status string, next time.Time) error {
	return s.journal.PublishOutage(ctx, impact, status, next)
}

func (s *LocalOperationsStatus) PublicStatus(ctx context.Context) (OperationsPublicStatus, error) {
	return s.journal.PublicStatus(ctx)
}

// PublishOutage updates only unresolved public facts. It is an independent
// local owner capability: it requires the explicitly configured private
// journal under owned OS storage, and works without the application database.
func (j *OperationsJournal) PublishOutage(ctx context.Context, impact, status string, next time.Time) error {
	if status != "investigating" && status != "paused" && status != "recovering" {
		return ErrValidation
	}
	return j.publish(ctx, OperationsPublicStatus{Impact: impact, Status: status, NextUpdateAt: &next})
}

func (j *OperationsJournal) publish(ctx context.Context, update OperationsPublicStatus) error {
	switch update.Impact {
	case "service", "invitations_and_inference", "inference":
	default:
		return ErrValidation
	}
	public := j.publicJournal()
	return public.transaction(ctx, true, func(state *operationsJournalState) error {
		now := j.cfg.Clock().UTC()
		if update.NextUpdateAt != nil {
			next := update.NextUpdateAt.UTC()
			if !next.After(now) {
				return ErrValidation
			}
			update.NextUpdateAt = &next
		}
		update.PublishedAt = now
		if state.ID == "" {
			id, err := newID()
			if err != nil {
				return auth.ErrUnavailable
			}
			state.ID = id
		}
		state.Public = &update
		return nil
	})
}

func (j *OperationsJournal) publicJournal() OperationsJournal {
	public := *j
	public.name = "status"
	public.id = ""
	return public
}

func (j *OperationsJournal) PublicStatus(ctx context.Context) (OperationsPublicStatus, error) {
	var status OperationsPublicStatus
	public := j.publicJournal()
	err := public.transaction(ctx, false, func(state *operationsJournalState) error {
		if state.Public == nil {
			return ErrNotFound
		}
		status = *state.Public
		return nil
	})
	return status, err
}

// PublishVerifiedRecovery runs through the current owner session and shared
// control fence. An independent outage publisher cannot assert recovery.
func (o *Operations) PublishVerifiedRecovery(ctx context.Context, p auth.Principal, reference, limitation string) error {
	if !validOperation(reference) || limitation != "" && limitation != "limited_pilot" {
		return ErrValidation
	}
	tx, err := o.supportTx(ctx, p, true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	controls, err := controlStatus(ctx, tx)
	if err != nil {
		return err
	}
	var verified bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_control_recovery r
	  JOIN careeros_control_commands c ON c.actor_id=r.actor_id AND c.operation_id=r.operation_id
	  WHERE r.reference=$1 AND (convert_from(c.result,'UTF8')::jsonb->>'version')::bigint=$2
	  AND (convert_from(r.evidence,'UTF8')::jsonb->>'wholeSystemExcluded')='false')`, reference, controls.Version).Scan(&verified)
	if err != nil {
		return unavailable(err)
	}
	if !verified || controls.InferenceStopped || controls.InvitationStopped {
		return auth.ErrForbidden
	}
	return o.journal.transaction(ctx, false, func(state *operationsJournalState) error {
		now := o.journal.cfg.Clock().UTC()
		for _, record := range state.Records {
			if stop, _ := operationsStop(record, now); stop {
				return auth.ErrForbidden
			}
		}
		if err := supportActor(ctx, tx, p, true); err != nil {
			return err
		}
		public := o.journal.publicJournal()
		return public.transaction(ctx, true, func(status *operationsJournalState) error {
			if status.Public == nil || status.Public.Status == "recovered" {
				return ErrConflict
			}
			status.Public = &OperationsPublicStatus{Impact: status.Public.Impact, Status: "recovered", Limitation: limitation, PublishedAt: now}
			return nil
		})
	})
}

type OperationsAbsence struct {
	Starts, Ends, NextUpdateAt time.Time
	PreparedCover, RightsOwner bool
}
type OperationsAbsenceResult struct {
	Paused, ActivePilotAllowed bool
}

// PlanAbsence applies containment immediately for a missing cover plan.
// It never manipulates grants, lifecycle clocks or rights authority.
func (o *Operations) PlanAbsence(ctx context.Context, p auth.Principal, plan OperationsAbsence) (OperationsAbsenceResult, error) {
	var result OperationsAbsenceResult
	tx, err := o.supportTx(ctx, p, true)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	now := o.journal.cfg.Clock().UTC()
	if plan.Starts.Before(now) || !plan.Ends.After(plan.Starts) || !plan.NextUpdateAt.After(now) {
		return result, ErrValidation
	}
	result = OperationsAbsenceResult{Paused: !plan.PreparedCover || !plan.RightsOwner, ActivePilotAllowed: plan.RightsOwner}
	if !result.Paused {
		return result, nil
	}
	if err = supportActor(ctx, tx, p, true); err != nil {
		return result, err
	}
	response := "maintain_rights_handling_and_review_before_reopening"
	if !plan.RightsOwner {
		response = "no_active_pilot_without_an_incident_and_rights_owner"
	}
	incident := OperationsIncident{Kind: "operator_absence", EventAt: now, Severity: "critical", State: "unacknowledged",
		Owner: o.cfg.Owner, Response: response}
	if err = o.journal.record(ctx, "absence/"+plan.Starts.UTC().Format(time.RFC3339Nano), incident); err != nil {
		return result, err
	}
	if err = o.journal.PublishOutage(ctx, "invitations_and_inference", "paused", plan.NextUpdateAt); err != nil {
		return result, err
	}
	if err = tx.Rollback(); err != nil {
		return result, unavailable(err)
	}
	return result, errors.Join(o.synchronizeContainment(ctx), o.Deliver(ctx))
}

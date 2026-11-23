package accountstore

import (
	"context"
	"database/sql"
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
type operationsPublicEpisode struct {
	ID         string    `json:"id"`
	RecordedAt time.Time `json:"recordedAt"`
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

func (s *LocalOperationsStatus) BeginOutage(ctx context.Context, impact, status string, next time.Time) error {
	return s.journal.publishOutage(ctx, impact, status, next, true, nil)
}

// Episode is available only to explicitly configured local operators.
func (s *LocalOperationsStatus) Episode(ctx context.Context) (string, error) {
	return s.journal.activeEpisode(ctx)
}

func (s *Store) ActiveOperationsOutage(ctx context.Context) (string, error) {
	ops := s.LocalOperationsObserver()
	if ops == nil {
		return "", nil
	}
	return ops.journal.activeEpisode(ctx)
}

func (j *OperationsJournal) activeEpisode(ctx context.Context) (string, error) {
	public := j.publicJournal()
	var id string
	err := public.transaction(ctx, false, func(state *operationsJournalState) error {
		if state.Public != nil && state.Public.Status != "recovered" && state.PublicEpisode != nil {
			id = state.PublicEpisode.ID
		}
		return nil
	})
	return id, err
}

func (s *LocalOperationsStatus) PublicStatus(ctx context.Context) (OperationsPublicStatus, error) {
	return s.journal.PublicStatus(ctx)
}

// PublishOutage updates only unresolved public facts. It is an independent
// local owner capability: it requires the explicitly configured private
// journal under owned OS storage, and works without the application database.
func (j *OperationsJournal) PublishOutage(ctx context.Context, impact, status string, next time.Time) error {
	return j.publishOutage(ctx, impact, status, next, false, nil)
}

func (j *OperationsJournal) publishOutage(ctx context.Context, impact, status string, next time.Time, newEpisode bool, authorize func() error) error {
	if status != "investigating" && status != "paused" && status != "recovering" {
		return ErrValidation
	}
	return j.publish(ctx, OperationsPublicStatus{Impact: impact, Status: status, NextUpdateAt: &next}, newEpisode, authorize)
}

func (j *OperationsJournal) publish(ctx context.Context, update OperationsPublicStatus, newEpisode bool, authorize func() error) error {
	switch update.Impact {
	case "service", "invitations_and_inference", "inference":
	default:
		return ErrValidation
	}
	public := j.publicJournal()
	return public.transaction(ctx, true, func(state *operationsJournalState) error {
		if authorize != nil {
			if err := authorize(); err != nil {
				return err
			}
		}
		now := j.cfg.Clock().UTC()
		if update.NextUpdateAt != nil {
			next := update.NextUpdateAt.UTC()
			if !next.After(now) {
				return ErrValidation
			}
			update.NextUpdateAt = &next
		}
		if newEpisode || state.PublicEpisode == nil || state.Public == nil || state.Public.Status == "recovered" || state.Public.Impact != update.Impact {
			id, err := newID()
			if err != nil {
				return auth.ErrUnavailable
			}
			// PostgreSQL recovery receipts use the physical local clock.
			// PublishedAt may use an injected exercise clock.
			state.PublicEpisode = &operationsPublicEpisode{ID: id, RecordedAt: time.Now().UTC()}
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
	return o.PublishVerifiedRecoveryWithServiceProof(ctx, p, reference, limitation, "")
}

func (o *Operations) PublishVerifiedRecoveryWithServiceProof(ctx context.Context, p auth.Principal, reference, limitation, serviceProof string) error {
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
	if controls.InferenceStopped {
		return auth.ErrForbidden
	}
	return o.journal.transaction(ctx, false, func(state *operationsJournalState) error {
		if err := o.journal.checkPendingFence(); err != nil {
			return err
		}
		now := o.journal.cfg.Clock().UTC()
		for _, record := range state.Records {
			if stop, _ := operationsStop(record, now); stop {
				return auth.ErrForbidden
			}
		}
		public := o.journal.publicJournal()
		return public.transaction(ctx, true, func(status *operationsJournalState) error {
			if err := o.journal.verifyWriteHealth(); err != nil {
				return err
			}
			if err := supportActor(ctx, tx, p, true); err != nil {
				return err
			}
			if status.Public == nil || status.PublicEpisode == nil || status.Public.Status == "recovered" {
				return ErrConflict
			}
			both := status.Public.Impact != "inference"
			if both && controls.InvitationStopped {
				return auth.ErrForbidden
			}
			episode := status.PublicEpisode
			if status.Public.Impact == "service" {
				if !validOperation(serviceProof) || o.cfg.VerifyServiceRecovery == nil {
					return auth.ErrForbidden
				}
				at, err := o.cfg.VerifyServiceRecovery(ctx, serviceProof, episode.ID)
				if err != nil || !at.After(episode.RecordedAt) || at.After(time.Now().UTC()) {
					return auth.ErrForbidden
				}
			} else if serviceProof != "" {
				return ErrValidation
			}
			if err := verifyEpisodeRecovery(ctx, tx, controls.Version, reference, episode.ID, episode.RecordedAt, both); err != nil {
				return err
			}
			if err := supportActor(ctx, tx, p, true); err != nil {
				return err
			}
			status.Public = &OperationsPublicStatus{Impact: status.Public.Impact, Status: "recovered", Limitation: limitation, PublishedAt: now}
			return nil
		})
	})
}

func verifyEpisodeRecovery(ctx context.Context, tx *sql.Tx, version int64, reference, episode string, since time.Time, both bool) error {
	var verified bool
	err := tx.QueryRowContext(ctx, `WITH commands AS (
	  SELECT actor_id,operation_id,convert_from(result,'UTF8')::jsonb AS receipt FROM careeros_control_commands
	), proofs AS (
	  SELECT r.reference,r.recorded_at,
	    convert_from(r.evidence,'UTF8')::jsonb->>'control' AS control,
	    (c.receipt->>'version')::bigint AS version
	  FROM careeros_control_recovery r JOIN commands c
	  ON c.actor_id=r.actor_id AND c.operation_id=r.operation_id
	  WHERE r.recorded_at>$1 AND (convert_from(r.evidence,'UTF8')::jsonb->>'wholeSystemExcluded')='false'
	  AND (convert_from(r.evidence,'UTF8')::jsonb->>'outageEpisode')=$5
	), applicable AS (
	  SELECT * FROM proofs p WHERE p.control IN ('inference','invitations') AND NOT EXISTS(
	    SELECT 1 FROM careeros_operations_containment h WHERE h.created_at>=p.recorded_at
	    AND (p.control='inference' OR h.invitation_stopped))
	  AND NOT EXISTS(SELECT 1 FROM commands c WHERE (c.receipt->>'version')::bigint>p.version
	    AND CASE p.control WHEN 'inference' THEN (c.receipt->>'inferenceStopped')::boolean
	      ELSE (c.receipt->>'invitationStopped')::boolean END)
	)
	SELECT EXISTS(SELECT 1 FROM applicable p WHERE reference=$2 AND version<=$3 AND (control='inference' OR $4)
	  AND NOT EXISTS(SELECT 1 FROM applicable latest WHERE latest.control=p.control AND latest.version>p.version))
	  AND EXISTS(SELECT 1 FROM applicable WHERE control='inference')
	  AND (NOT $4 OR EXISTS(SELECT 1 FROM applicable WHERE control='invitations'))`, since, reference, version, both, episode).Scan(&verified)
	if err != nil {
		return unavailable(err)
	}
	if !verified {
		return auth.ErrForbidden
	}
	return nil
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
	covered := plan.PreparedCover && plan.RightsOwner && o.cfg.VerifyPreparedCover != nil && o.cfg.VerifyPreparedCover(ctx, plan) == nil
	result = OperationsAbsenceResult{Paused: !covered, ActivePilotAllowed: plan.RightsOwner}
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
	key := "absence/" + plan.Starts.UTC().Format(time.RFC3339Nano)
	if err = o.journal.containmentTransaction(ctx, func(state *operationsJournalState) error {
		if err := supportActor(ctx, tx, p, true); err != nil {
			return err
		}
		now := o.journal.cfg.Clock().UTC()
		if plan.Starts.Before(now) || !plan.NextUpdateAt.After(now) {
			return ErrValidation
		}
		incident.EventAt = now
		o.journal.upsert(state, key, incident, false)
		id := o.journal.binding([]byte("incident\x00" + key))
		record := state.Records[id]
		if !record.AbsenceUntil.IsZero() && !record.AbsenceUntil.Equal(plan.Ends) {
			return ErrConflict
		}
		record.AbsenceStarts, record.AbsenceUntil = plan.Starts.UTC(), plan.Ends.UTC()
		state.Records[id] = record
		return nil
	}); err != nil {
		if errors.Is(err, auth.ErrUnavailable) {
			_ = tx.Rollback()
			return result, errors.Join(err, o.synchronizeEmergencyStop(ctx))
		}
		return result, err
	}
	publishErr := o.journal.publishOutage(ctx, "invitations_and_inference", "paused", plan.NextUpdateAt, true, func() error { return supportActor(ctx, tx, p, true) })
	if err = tx.Rollback(); err != nil {
		return result, unavailable(err)
	}
	return result, errors.Join(publishErr, o.synchronizeContainment(ctx), o.Deliver(ctx))
}

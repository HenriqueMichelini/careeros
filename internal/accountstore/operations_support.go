package accountstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"professional-information-repo/internal/auth"
)

const (
	diagnosticTTL = 7 * 24 * time.Hour
	casesTTL      = 30 * 24 * time.Hour
	necessityDue  = 7 * 24 * time.Hour
	responseDue   = 48 * time.Hour
)

var ErrOperationsSpecialPurpose = errors.New("use the separately authorized purpose-specific procedure")

// Intake accepts the necessary request and a deliberately minimized example.
// The ingress adapter must omit attachments before calling this command.
type OperationsCaseInput struct {
	Purpose     string    `json:"purpose"`
	Locale      string    `json:"locale"`
	Message     string    `json:"message"`
	Example     string    `json:"example,omitempty"`
	ExampleKind string    `json:"exampleKind,omitempty"`
	ReceivedAt  time.Time `json:"receivedAt"`
}

type OperationsCase struct {
	OperationsCaseInput
	ID                string     `json:"id"`
	Owner             string     `json:"owner"`
	Status            string     `json:"status"`
	Response          string     `json:"response,omitempty"`
	IntakeAt          time.Time  `json:"intakeAt"`
	FirstResponseDue  time.Time  `json:"internalFirstResponseDue"`
	NecessityReviewAt time.Time  `json:"necessityReviewAt"`
	RespondedAt       *time.Time `json:"respondedAt,omitempty"`
	ClosedAt          *time.Time `json:"closedAt,omitempty"`
}

func (o *Operations) supportTx(ctx context.Context, p auth.Principal, owner bool) (*sql.Tx, error) {
	tx, err := o.store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	if err = LockControlTransaction(ctx, tx); err == nil {
		err = supportActor(ctx, tx, p, owner)
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func supportActor(ctx context.Context, tx *sql.Tx, p auth.Principal, owner bool) error {
	// Restricted compromised sessions must not gain ordinary support access.
	if err := authorizedAccount(ctx, tx, p); err != nil {
		return err
	}
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_operations_roles
	  WHERE account_id=$1 AND (role='owner' OR NOT $2 AND role='support'))`, p.AccountID, owner).Scan(&allowed)
	if err != nil {
		return unavailable(err)
	}
	if !allowed {
		return auth.ErrForbidden
	}
	return nil
}

func permittedSupportText(s string, limit int) bool {
	return len(s) <= limit && utf8.ValidString(s) && strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0)
}

func specialPurpose(purpose string) bool {
	switch purpose {
	case "rights", "privacy", "security", "financial", "fiscal", "deletion", "terminal_replay", "expense_reconciliation":
		return true
	}
	return false
}

func (o *Operations) CreateCase(ctx context.Context, p auth.Principal, input OperationsCaseInput) (OperationsCase, error) {
	var c OperationsCase
	tx, err := o.supportTx(ctx, p, false)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	if specialPurpose(input.Purpose) {
		return c, ErrOperationsSpecialPurpose
	}
	switch input.Purpose {
	case "product", "account", "billing", "voluntary_feedback":
	default:
		return c, ErrValidation
	}
	if input.Locale != "en" && input.Locale != "pt-BR" || !permittedSupportText(input.Message, 4096) ||
		input.Example == "" && input.ExampleKind != "" || input.Example != "" &&
		(!permittedSupportText(input.Example, 512) || input.ExampleKind != "redacted" && input.ExampleKind != "synthetic") {
		return c, ErrValidation
	}
	id, err := newID()
	if err != nil {
		return c, unavailable(err)
	}
	now := o.journal.cfg.Clock().UTC()
	received := input.ReceivedAt.UTC()
	if received.IsZero() || received.After(now) {
		return c, ErrValidation
	}
	input.ReceivedAt = received
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_operations_cases
	  (id,purpose,locale,message,example,example_kind,received_at,intake_at,review_at)
	  VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, input.Purpose, input.Locale, input.Message, input.Example, input.ExampleKind, received, now, now.Add(necessityDue)); err != nil {
		return c, unavailable(err)
	}
	if err = supportActor(ctx, tx, p, false); err != nil {
		return c, err
	}
	c = OperationsCase{OperationsCaseInput: input, ID: id, Owner: o.cfg.Owner, Status: "received",
		IntakeAt: now, FirstResponseDue: received.Add(responseDue), NecessityReviewAt: now.Add(necessityDue)}
	return c, unavailable(tx.Commit())
}

func (o *Operations) readCase(ctx context.Context, tx *sql.Tx, id string, held bool) (OperationsCase, error) {
	var c OperationsCase
	if len(id) != 32 || !validOperation(id) {
		return c, ErrValidation
	}
	err := tx.QueryRowContext(ctx, `SELECT c.id,c.purpose,c.locale,c.message,c.example,c.example_kind,c.response,
	  c.received_at,COALESCE(c.intake_at,c.received_at),c.review_at,c.responded_at,c.closed_at FROM careeros_operations_cases c
	  WHERE c.id=$1 AND (CASE WHEN $3 THEN EXISTS(
	    SELECT 1 FROM careeros_operations_case_holds h WHERE h.case_id=c.id AND h.retired_at IS NULL)
	    ELSE c.closed_at IS NULL OR c.closed_at>$2 END) FOR UPDATE`,
		id, o.journal.cfg.Clock().UTC().Add(-casesTTL), held).Scan(
		&c.ID, &c.Purpose, &c.Locale, &c.Message, &c.Example, &c.ExampleKind, &c.Response,
		&c.ReceivedAt, &c.IntakeAt, &c.NecessityReviewAt, &c.RespondedAt, &c.ClosedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, unavailable(err)
	}
	if !held && c.ClosedAt != nil && !o.journal.cfg.Clock().UTC().Before(c.ClosedAt.Add(casesTTL)) {
		return OperationsCase{}, ErrNotFound
	}
	c.ReceivedAt, c.IntakeAt, c.NecessityReviewAt = c.ReceivedAt.UTC(), c.IntakeAt.UTC(), c.NecessityReviewAt.UTC()
	c.Owner, c.Status, c.FirstResponseDue = o.cfg.Owner, "received", c.ReceivedAt.Add(responseDue)
	if c.RespondedAt != nil {
		at := c.RespondedAt.UTC()
		c.RespondedAt = &at
		c.Status = "responded"
	}
	if c.ClosedAt != nil {
		at := c.ClosedAt.UTC()
		c.ClosedAt = &at
		c.Status = "closed"
	}
	return c, nil
}

func (o *Operations) WithCase(ctx context.Context, p auth.Principal, id string, held bool, write func(OperationsCase) error) error {
	if write == nil {
		return ErrValidation
	}
	tx, err := o.supportTx(ctx, p, held)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	c, err := o.readCase(ctx, tx, id, held)
	if err != nil {
		return err
	}
	if err = supportActor(ctx, tx, p, held); err != nil {
		return err
	}
	return write(c)
}

func (o *Operations) Case(ctx context.Context, p auth.Principal, id string) (OperationsCase, error) {
	var result OperationsCase
	err := o.WithCase(ctx, p, id, false, func(c OperationsCase) error { result = c; return nil })
	return result, err
}

func (o *Operations) UpdateCase(ctx context.Context, p auth.Principal, id, action, response string) error {
	owner := action == "review"
	tx, err := o.supportTx(ctx, p, owner)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	c, err := o.readCase(ctx, tx, id, false)
	if err != nil {
		return err
	}
	now := o.journal.cfg.Clock().UTC()
	switch action {
	case "respond":
		if c.ClosedAt != nil || !permittedSupportText(response, 4096) {
			return ErrValidation
		}
		_, err = tx.ExecContext(ctx, `UPDATE careeros_operations_cases SET response=$2,
		  responded_at=COALESCE(responded_at,$3) WHERE id=$1`, id, response, now)
	case "close":
		_, err = tx.ExecContext(ctx, `UPDATE careeros_operations_cases SET closed_at=COALESCE(closed_at,$2) WHERE id=$1`, id, now)
	case "review":
		if c.ClosedAt != nil {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE careeros_operations_cases SET review_at=$2 WHERE id=$1`, id, now.Add(necessityDue))
	default:
		return ErrValidation
	}
	if err != nil {
		return unavailable(err)
	}
	if err = supportActor(ctx, tx, p, owner); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

func (o *Operations) ReviewCaseHold(ctx context.Context, p auth.Principal, id, purpose string, next time.Time, retire bool) error {
	tx, err := o.supportTx(ctx, p, true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = o.readCase(ctx, tx, id, true)
	existing := err == nil
	if errors.Is(err, ErrNotFound) && !retire {
		_, err = o.readCase(ctx, tx, id, false)
	}
	if err != nil {
		return err
	}
	now := o.journal.cfg.Clock().UTC()
	if retire {
		_, err = tx.ExecContext(ctx, `UPDATE careeros_operations_case_holds SET retired_at=$2 WHERE case_id=$1 AND retired_at IS NULL`, id, now)
	} else {
		if !specialPurpose(purpose) || !next.After(now) {
			return ErrValidation
		}
		if existing {
			result, updateErr := tx.ExecContext(ctx, `UPDATE careeros_operations_case_holds SET review_at=$3
			  WHERE case_id=$1 AND purpose=$2 AND retired_at IS NULL`, id, purpose, next)
			if updateErr != nil {
				return unavailable(updateErr)
			}
			count, countErr := result.RowsAffected()
			if countErr != nil {
				return unavailable(countErr)
			}
			if count != 1 {
				return ErrConflict
			}
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO careeros_operations_case_holds(case_id,purpose,review_at)
			  VALUES($1,$2,$3) ON CONFLICT(case_id) DO UPDATE SET purpose=excluded.purpose,
			  review_at=excluded.review_at,retired_at=NULL`, id, purpose, next)
		}
	}
	if err != nil {
		return unavailable(err)
	}
	if err = supportActor(ctx, tx, p, true); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

func (o *Operations) PruneOrdinaryRecords(ctx context.Context, p auth.Principal) error {
	tx, err := o.supportTx(ctx, p, true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := o.journal.cfg.Clock().UTC()
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_operations_diagnostics WHERE event_at<=$1`, now.Add(-diagnosticTTL)); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_operations_cases c WHERE c.closed_at<=$1
	  AND NOT EXISTS(SELECT 1 FROM careeros_operations_case_holds h WHERE h.case_id=c.id AND h.retired_at IS NULL)`, now.Add(-casesTTL)); err != nil {
		return unavailable(err)
	}
	if err = supportActor(ctx, tx, p, true); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

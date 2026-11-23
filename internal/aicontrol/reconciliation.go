package aicontrol

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

//go:embed reconciliation.sql
var reconciliationSchema string

const ReconciliationPolicy = "restricted-reconciliation-local-20261104-v1"

// ReconciliationEvidence is normalized non-content evidence. Source and Reason
// are bounded references/codes, never invoice text or arbitrary operator notes.
// Difference is an attributable upper bound, not a percentage safety buffer.
// Only the elevated operator can submit this evidence. Runtime uses the separate
// saved-observation validator and cannot select this value or a lower expense.
type ReconciliationEvidence struct {
	Source             string     `json:"source"`
	Revision           int64      `json:"revision"`
	Supersedes         string     `json:"supersedes,omitempty"`
	Reason             string     `json:"reason"`
	Provider           string     `json:"provider"`
	Model              string     `json:"model"`
	Tier               string     `json:"tier"`
	Rate               string     `json:"rate"`
	Currency           string     `json:"currency"`
	BillingNamespace   string     `json:"billingNamespace"`
	RequestID          string     `json:"requestId"`
	Complete           bool       `json:"complete"`
	ValuationReference string     `json:"valuationReference,omitempty"`
	Usage              *Usage     `json:"usage,omitempty"`
	Difference         *int64     `json:"difference,omitempty"`
	Gross              *int64     `json:"gross,omitempty"`
	EventAt            *time.Time `json:"eventAt,omitempty"`
}

type ReconciliationCommand struct {
	RateRevision            *RecordedRateRevision   `json:"rateRevision,omitempty"`
	StopVersion             int64                   `json:"stopVersion,omitempty"`
	BoundsReference         string                  `json:"boundsReference,omitempty"`
	ReconciliationReference string                  `json:"reconciliationReference,omitempty"`
	ID                      string                  `json:"id"`
	Kind                    string                  `json:"kind"`
	Dispatch                string                  `json:"dispatch"`
	ExpectedVersion         int64                   `json:"expectedVersion"`
	Evidence                ReconciliationEvidence  `json:"evidence"`
	Preview                 string                  `json:"preview,omitempty"`
	Retention               *DetailRetirementReview `json:"retention,omitempty"`
}

type ReconciliationPreview struct {
	Unpriced   bool   `json:"unpriced,omitempty"`
	Before     int64  `json:"before"`
	After      int64  `json:"after"`
	Version    int64  `json:"version"`
	Category   string `json:"category"`
	Token      string `json:"token"`
	Amount     *int64 `json:"amount,omitempty"`
	Difference *int64 `json:"difference,omitempty"`
}

type ReconciliationStatus struct {
	Unpriced         bool                  `json:"unpriced"`
	Expense          *int64                `json:"expense,omitempty"`
	Floor            int64                 `json:"floor"`
	AttributionFloor int64                 `json:"attributionFloor"`
	Dispatch         string                `json:"dispatch"`
	Counted          int64                 `json:"counted"`
	Maximum          int64                 `json:"maximum"`
	Category         string                `json:"category"`
	Remote           string                `json:"remote"`
	Version          int64                 `json:"version"`
	Compacted        bool                  `json:"compacted"`
	AdmissionStopped bool                  `json:"admissionStopped"`
	NextReview       *time.Time            `json:"nextReview,omitempty"`
	Scopes           []ReconciliationScope `json:"scopes"`
}
type ReconciliationScope struct {
	Name     string
	Counted  int64
	Occupied int
}

// Reconciliation is an explicit local-only elevated command composition. Its
// role-bound pool cannot read Profile/content tables. A caller principal never
// becomes the target owner; target linkage is resolved under the ledger locks.
type Reconciliation struct {
	h                  *Harness
	runtime            bool
	observer           bool
	checkpointVerifier func(context.Context, string, string) error
}

func (h *Harness) Reconciliation() *Reconciliation { return &Reconciliation{h: h} }

// InstallLocalReconciliation is a migrator-only bootstrap in an already named
// disposable schema. One owner is installed; commands cannot grant or elevate
// themselves. Runtime/reconciliation pools do not have DML on this authority.
func InstallLocalReconciliation(ctx context.Context, migrator *sql.DB, namespace, actor string, expires time.Time) error {
	if migrator == nil || !tokenLabel.MatchString(namespace) || !tokenLabel.MatchString(actor) || !expires.After(time.Now()) || expires.After(time.Now().Add(24*time.Hour)) {
		return ErrDenied
	}
	tx, err := migrator.BeginTx(ctx, nil)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback()
	var schema string
	if tx.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema) != nil || !strings.HasPrefix(schema, "careeros_ai_test_") {
		return ErrDenied
	}
	if _, err = tx.ExecContext(ctx, `SELECT id FROM ai_fixture_namespace WHERE id=$1 FOR UPDATE`, namespace); err != nil {
		return ErrUnavailable
	}
	if err = ensureAttributionFloor(ctx, tx); err != nil {
		return err
	}
	var count int
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_recon_operator WHERE namespace=$1 AND actor<>$2`, namespace, actor).Scan(&count) != nil || count != 0 {
		return ErrDenied
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_recon_operator(namespace,actor,expires_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, namespace, actor, expires)
	if err != nil {
		return ErrUnavailable
	}
	return safe(tx.Commit())
}

// A distinct, code-owned local audit key keeps numeric/replay verification
// independent of input-binding rotation. This fixed public fixture key is not a
// production credential or an enablement path.
func (r *Reconciliation) digest(actor, purpose string, value any) string {
	raw, _ := json.Marshal(value)
	key := sha256.Sum256([]byte("synthetic-reconciliation-audit-key-v1"))
	m := hmac.New(sha256.New, key[:])
	m.Write([]byte(r.h.cfg.Namespace + "\x00" + actor + "\x00" + purpose + "\x00"))
	m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}
func (r *Reconciliation) begin(ctx context.Context, p auth.Principal, id string, apply bool) (*sql.Tx, Dispatch, int64, error) {
	if r == nil || r.h == nil || r.h.reviewerDB == nil || !tokenLabel.MatchString(id) {
		return nil, Dispatch{}, 0, ErrDenied
	}
	tx, err := r.h.reviewerDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, Dispatch{}, 0, ErrUnavailable
	}
	fail := func(e error) (*sql.Tx, Dispatch, int64, error) { tx.Rollback(); return nil, Dispatch{}, 0, e }
	if apply {
		var enrolled bool
		if tx.QueryRowContext(ctx, `SELECT careeros_operations_required()`).Scan(&enrolled) != nil {
			return fail(ErrUnavailable)
		}
		if enrolled {
			if err = accountstore.LockControlStopTransaction(ctx, tx); err != nil {
				return fail(err)
			}
		}
	}
	if !r.observer {
		if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
			return fail(err)
		}
	}
	// Same namespace lock order as admission. Financial evidence remains writable
	// after containment, retirement, customer closure or expired target grants.
	if _, err = tx.ExecContext(ctx, `SELECT id FROM ai_fixture_namespace WHERE id=$1 FOR UPDATE`, r.h.cfg.Namespace); err != nil {
		return fail(ErrUnavailable)
	}
	var allowed bool
	if !r.runtime && (tx.QueryRowContext(ctx, `SELECT NOT revoked AND expires_at>clock_timestamp() FROM ai_recon_operator WHERE namespace=$1 AND actor=$2 FOR SHARE`, r.h.cfg.Namespace, p.AccountID).Scan(&allowed) != nil || !allowed) {
		return fail(ErrDenied)
	}
	var action string
	if tx.QueryRowContext(ctx, `SELECT action FROM ai_fixture_dispatch WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, id).Scan(&action) != nil {
		return fail(ErrDenied)
	}
	if _, err = tx.ExecContext(ctx, `SELECT id FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, r.h.cfg.Namespace, action); err != nil {
		return fail(ErrUnavailable)
	}
	if r.runtime {
		var owner string
		if tx.QueryRowContext(ctx, `SELECT owner FROM ai_fixture_action WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, action).Scan(&owner) != nil || owner != p.AccountID {
			return fail(ErrDenied)
		}
	}
	if _, err = tx.ExecContext(ctx, `SELECT id FROM ai_fixture_dispatch WHERE namespace=$1 AND id=$2 FOR UPDATE`, r.h.cfg.Namespace, id); err != nil {
		return fail(ErrUnavailable)
	}
	d, err := loadDispatch(ctx, tx, r.h.cfg.Namespace, id)
	if err != nil {
		return fail(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_recon_state(namespace,dispatch) VALUES($1,$2) ON CONFLICT DO NOTHING`, r.h.cfg.Namespace, id); err != nil {
		return fail(ErrUnavailable)
	}
	var version int64
	if tx.QueryRowContext(ctx, `SELECT version FROM ai_recon_state WHERE namespace=$1 AND dispatch=$2 FOR UPDATE`, r.h.cfg.Namespace, id).Scan(&version) != nil {
		return fail(ErrUnavailable)
	}
	if err = r.authorize(ctx, tx, p); err != nil {
		return fail(err)
	}
	return tx, d, version, nil
}
func (r *Reconciliation) authorize(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	if r.observer {
		return nil
	}
	if err := accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return err
	}
	if r.runtime {
		return nil
	}
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT NOT revoked AND expires_at>clock_timestamp() FROM ai_recon_operator WHERE namespace=$1 AND actor=$2`, r.h.cfg.Namespace, p.AccountID).Scan(&valid) != nil || !valid {
		return ErrDenied
	}
	return nil
}
func (r *Reconciliation) normalized(c ReconciliationCommand) (ReconciliationCommand, error) {
	c.Preview = ""
	e := c.Evidence
	if !tokenLabel.MatchString(c.ID) || !tokenLabel.MatchString(c.Dispatch) || c.ExpectedVersion < 0 || !tokenLabel.MatchString(e.Source) || !tokenLabel.MatchString(e.Reason) || e.Revision <= 0 || e.Supersedes != "" && !tokenLabel.MatchString(e.Supersedes) || !tokenLabel.MatchString(e.BillingNamespace) || !metadataLabel.MatchString(e.RequestID) {
		return c, ErrDenied
	}
	if e.EventAt != nil && (e.EventAt.IsZero() || e.EventAt.After(time.Now().Add(time.Minute))) {
		return c, ErrDenied
	}
	if e.ValuationReference != "" && !tokenLabel.MatchString(e.ValuationReference) {
		return c, ErrDenied
	}
	if c.Kind != "compact" && c.Retention != nil || c.Kind != "rate_revision" && c.RateRevision != nil {
		return c, ErrDenied
	}
	if c.Kind != "reopen" && (c.StopVersion != 0 || c.BoundsReference != "" || c.ReconciliationReference != "") {
		return c, ErrDenied
	}
	if c.Kind == "usage" && e.Gross != nil || (c.Kind == "bill" || c.Kind == "corrected_bill") && e.Usage != nil {
		return c, ErrDenied
	}
	if c.Kind == "observation" && (e.Gross != nil || e.Difference != nil) {
		return c, ErrDenied
	}
	if (c.Kind == "maximum" || c.Kind == "compact") && (e.Gross != nil || e.Difference != nil || e.Usage != nil) {
		return c, ErrDenied
	}
	switch c.Kind {
	case "observation", "usage", "maximum", "bill", "corrected_bill", "compact", "terminal", "unpriced", "reopen", "rate_revision":
	default:
		return c, ErrDenied
	}
	return c, nil
}
func (r *Reconciliation) effect(ctx context.Context, tx *sql.Tx, d Dispatch, c ReconciliationCommand) (ReconciliationPreview, error) {
	out := ReconciliationPreview{Before: d.Counted, After: d.Counted, Category: d.Category}
	e := c.Evidence
	if d.IntentAt == nil || d.State == "reserved" || d.State == "aborted" || e.Provider != d.Provider {
		return out, ErrDenied
	}
	var provider, model, tier, rate, currency, billing string
	if tx.QueryRowContext(ctx, `SELECT provider,model,tier,rate_id,currency,billing_namespace FROM ai_recon_contract WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID).Scan(&provider, &model, &tier, &rate, &currency, &billing) != nil {
		return out, ErrDenied
	}
	if e.Provider != provider || e.Model != model || e.Tier != tier || e.BillingNamespace != billing {
		return out, ErrDenied
	}
	if c.Kind == "unpriced" {
		if !tokenLabel.MatchString(e.Rate) || len(e.Currency) != 3 || e.Gross != nil && *e.Gross < 0 {
			return out, ErrDenied
		}
		for _, c := range e.Currency {
			if c < 'A' || c > 'Z' {
				return out, ErrDenied
			}
		}
		out.After = countMax(d.Counted, d.Maximum)
		out.Unpriced = true
		return out, nil
	}
	if e.Rate != rate || e.Currency != currency {
		return out, ErrDenied
	}
	if c.Kind == "rate_revision" {
		return out, r.validateRateRevision(ctx, tx, d, c)
	}
	if c.Kind == "reopen" {
		return out, r.validateReopen(ctx, tx, d, c)
	}
	if c.Kind == "terminal" {
		return out, r.validateTerminal(ctx, tx, d, c.Evidence)
	}
	if c.Kind == "compact" {
		return out, r.validateRetirement(d, c.Retention)
	}
	if c.Kind == "observation" {
		if e.Usage != nil {
			var amount int64
			var err error
			if rate == QualificationRateID {
				amount, err = QualificationEstimate(e.Provider, e.Model, e.Tier, rate, *e.Usage)
			} else {
				amount, err = Estimate(e.Provider, e.Model, e.Tier, *e.Usage)
			}
			if err != nil {
				return out, err
			}
			out.After = countMax(d.Counted, amount)
			out.Amount = &amount
		}
		return out, nil
	}
	amount := d.Maximum
	category := "reviewed_conservative_maximum"
	switch c.Kind {
	case "usage":
		if e.Usage == nil {
			return out, ErrDenied
		}
		var err error
		if e.Rate == QualificationRateID {
			amount, err = QualificationEstimate(e.Provider, e.Model, e.Tier, e.Rate, *e.Usage)
		} else {
			amount, err = Estimate(e.Provider, e.Model, e.Tier, *e.Usage)
		}
		if err != nil {
			return out, err
		}
		category = "supported_estimate"
	case "bill", "corrected_bill":
		if e.Gross == nil || *e.Gross < 0 {
			return out, ErrDenied
		}
		amount = *e.Gross
		category = "confirmed_bill"
	case "maximum":
		now, healthy := r.h.cfg.Clock.Sample()
		if !healthy || now.Before(*d.IntentAt) || now.Sub(*d.IntentAt) < 168*time.Hour || d.Expense != nil {
			return out, ErrDenied
		}
	}
	var floor, revision, unpricedRevision, attributionFloor int64
	var source, command, unpricedCommand, unpricedSource, retentionReference string
	var unpriced bool
	if tx.QueryRowContext(ctx, `SELECT disputed_floor,source,source_revision,command_id,unpriced,unpriced_command,unpriced_source,unpriced_revision,attribution_floor,retention_reference FROM ai_recon_state WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID).Scan(&floor, &source, &revision, &command, &unpriced, &unpricedCommand, &unpricedSource, &unpricedRevision, &attributionFloor, &retentionReference) != nil {
		return out, ErrUnavailable
	}
	covered := e.Complete && e.Difference != nil && *e.Difference >= 0
	if c.Kind == "maximum" {
		covered = false
	}
	if e.Difference != nil && *e.Difference < 0 {
		return out, ErrDenied
	}
	contribution := countMax(d.Floor, amount)
	if covered {
		sum, err := checkedSum(amount, *e.Difference)
		if err != nil {
			return out, err
		}
		if sum < attributionFloor && sum < d.Counted {
			return out, ErrDenied
		}
		contribution = countMax(countMax(floor, attributionFloor), sum)
	}
	compatibleValuation := unpriced && c.Kind == "corrected_bill" && covered && e.Supersedes == unpricedCommand && e.Source == unpricedSource && e.Revision > unpricedRevision && tokenLabel.MatchString(e.ValuationReference)
	if unpriced && contribution < d.Counted && !compatibleValuation {
		return out, ErrDenied
	}
	out.Unpriced = unpriced && !compatibleValuation
	if d.Expense != nil {
		// Evidence ordering cannot be inferred from a locally larger version. A
		// lower contribution needs an explicit source revision superseding exactly
		// the accepted obligation and separate reviewed correction authority.
		if contribution < d.Counted {
			initialFloorSettlement := command == "" && d.Category == "supported_estimate" && c.Kind == "usage"
			finalBill := c.Kind == "bill" && d.Category == "supported_estimate" && e.Supersedes == command
			correctedBill := c.Kind == "corrected_bill" && e.Supersedes == command && e.Source == source && e.Revision > revision
			if (!initialFloorSettlement && !finalBill && !correctedBill && !compatibleValuation) || !covered {
				return out, ErrDenied
			}
		} else if e.Source == source && e.Revision < revision {
			return out, ErrDenied
		}
		if d.Category == "confirmed_bill" && c.Kind == "usage" {
			return out, ErrDenied
		}
	}
	if contribution < d.Counted && (d.Compacted || retentionReference != "") {
		retained, err := r.retainedAttribution(ctx, tx, d, command, source, revision, e)
		if err != nil {
			return out, err
		}
		if !retained {
			return out, ErrDenied
		}
	}
	if category != "reviewed_conservative_maximum" {
		out.Amount = &amount
	}
	out.Difference = e.Difference
	out.After = contribution
	out.Category = category
	return out, nil
}
func (r *Reconciliation) Preview(ctx context.Context, p auth.Principal, c ReconciliationCommand) (ReconciliationPreview, error) {
	c, err := r.normalized(c)
	if err != nil {
		return ReconciliationPreview{}, err
	}
	tx, d, v, err := r.begin(ctx, p, c.Dispatch, false)
	if err != nil {
		return ReconciliationPreview{}, err
	}
	defer tx.Rollback()
	if saved, found, err := r.priorEvidence(ctx, tx, c); err != nil {
		return saved, err
	} else if found {
		return saved, r.authorize(ctx, tx, p)
	}
	if c.ExpectedVersion != v {
		return ReconciliationPreview{}, ErrConflict
	}
	out, err := r.effect(ctx, tx, d, c)
	if err != nil {
		return out, err
	}
	out.Version = v
	actor, err := r.previewActor(ctx, tx, p)
	if err != nil {
		return out, err
	}
	out.Token = r.digest(actor, "preview", struct {
		Command ReconciliationCommand
		Result  ReconciliationPreview
	}{c, out})
	return out, r.authorize(ctx, tx, p)
}
func (r *Reconciliation) Apply(ctx context.Context, p auth.Principal, c ReconciliationCommand) (ReconciliationPreview, error) {
	token := c.Preview
	c, err := r.normalized(c)
	if err != nil {
		return ReconciliationPreview{}, err
	}
	tx, d, v, err := r.begin(ctx, p, c.Dispatch, true)
	if err != nil {
		return ReconciliationPreview{}, err
	}
	defer tx.Rollback()
	binding := r.digest(p.AccountID, "command", c)
	var old []byte
	var result ReconciliationPreview
	var saved []byte
	err = tx.QueryRowContext(ctx, `SELECT binding,result FROM ai_recon_command WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, c.ID).Scan(&old, &saved)
	if err == nil {
		if !hmac.Equal(old, decodeDigest(binding)) {
			return result, ErrConflict
		}
		if json.Unmarshal(saved, &result) != nil {
			return result, ErrUnavailable
		}
		return result, r.authorize(ctx, tx, p)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, ErrUnavailable
	}
	if saved, found, err := r.priorEvidence(ctx, tx, c); err != nil {
		return saved, err
	} else if found {
		return saved, r.authorize(ctx, tx, p)
	}
	if c.ExpectedVersion != v {
		return result, ErrConflict
	}
	result, err = r.effect(ctx, tx, d, c)
	if err != nil {
		return result, err
	}
	result.Version = v
	actor, err := r.previewActor(ctx, tx, p)
	if err != nil {
		return result, err
	}
	wanted := r.digest(actor, "preview", struct {
		Command ReconciliationCommand
		Result  ReconciliationPreview
	}{c, result})
	if !hmac.Equal([]byte(wanted), []byte(token)) {
		return result, ErrConflict
	}
	slots := 0
	if c.Kind == "terminal" && d.Remote != "terminal_proven" {
		slots = -1
	}
	if err = r.h.adjustScopes(ctx, tx, d.ID, result.After-d.Counted, slots); err != nil {
		return result, err
	}
	e := c.Evidence
	detail, _ := json.Marshal(e)
	result.Version++
	saved, _ = json.Marshal(result)
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_recon_command(namespace,id,dispatch,binding,actor,kind,source,source_revision,supersedes,reason,event_at,before_amount,after_amount,version,category,detail,result,evidence_binding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, r.h.cfg.Namespace, c.ID, d.ID, decodeDigest(binding), r.auditActor(p), c.Kind, e.Source, e.Revision, e.Supersedes, e.Reason, e.EventAt, result.Before, result.After, result.Version, result.Category, detail, saved, decodeDigest(r.evidenceDigest(c))); err != nil {
		return result, ErrUnavailable
	}
	if c.Kind == "compact" {
		if err = r.retireDetails(ctx, tx, d, c.Retention); err != nil {
			return result, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ai_recon_state SET version=$3,source=CASE WHEN $7 THEN $4 ELSE source END,source_revision=CASE WHEN $7 THEN $5 ELSE source_revision END,command_id=CASE WHEN $7 THEN $6 ELSE command_id END WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, result.Version, e.Source, e.Revision, c.ID, financialEvidence(c.Kind)); err != nil {
		return result, ErrUnavailable
	}
	if financialEvidence(c.Kind) {
		expense := result.Amount
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET floor_amount=$3,counted=$3,expense=$4,category=$5,evidence_version=evidence_version+1,maximum_breached=maximum_breached OR $3>maximum WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID, result.After, expense, result.Category); err != nil {
			return result, ErrUnavailable
		}
	}
	if c.Kind == "observation" && result.After > d.Counted {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET floor_amount=$3,counted=$3,maximum_breached=maximum_breached OR $3>maximum WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID, result.After); err != nil {
			return result, ErrUnavailable
		}
	}
	if c.Kind == "unpriced" {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_recon_state SET unpriced=true,unpriced_command=$3,unpriced_source=$4,unpriced_revision=$5 WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, c.ID, e.Source, e.Revision); err != nil {
			return result, ErrUnavailable
		}
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET floor_amount=$3,counted=$3 WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID, result.After); err != nil {
			return result, ErrUnavailable
		}
	} else if c.Kind == "corrected_bill" && !result.Unpriced {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_recon_state SET unpriced=false WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID); err != nil {
			return result, ErrUnavailable
		}
	}
	if c.Kind == "rate_revision" {
		if err = r.recordRateRevision(ctx, tx, c); err != nil {
			return result, err
		}
	}
	if c.Kind == "reopen" {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_recon_stop SET stopped=false,version=version+1 WHERE namespace=$1`, r.h.cfg.Namespace); err != nil {
			return result, ErrUnavailable
		}
	}
	if c.Kind == "terminal" {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET remote_state='terminal_proven' WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID); err != nil {
			return result, ErrUnavailable
		}
	}
	breach := (result.After > d.Maximum && (!d.MaximumBreached || result.After > d.Counted)) || c.Kind == "unpriced"
	if breach {
		contract := e.Provider + "/" + e.Model + "/" + e.Tier + "/" + e.Rate + "/" + e.Currency
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_recon_event(namespace,id,dispatch,contract,obligation,reserved) VALUES($1,$2,$3,$4,$5,$6)`, r.h.cfg.Namespace, c.ID, d.ID, contract, result.After, d.Maximum); err != nil {
			return result, ErrUnavailable
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_recon_stop(namespace,stopped,version,event_id,contract,obligation,reserved) VALUES($1,true,1,$2,$3,$4,$5) ON CONFLICT(namespace) DO UPDATE SET stopped=true,version=ai_recon_stop.version+1,event_id=EXCLUDED.event_id,contract=EXCLUDED.contract,obligation=EXCLUDED.obligation,reserved=EXCLUDED.reserved`, r.h.cfg.Namespace, c.ID, contract, result.After, d.Maximum); err != nil {
			return result, ErrUnavailable
		}
	}
	if err = r.h.fault("before_reconciliation_commit"); err != nil {
		return result, err
	}
	if err = r.authorize(ctx, tx, p); err != nil {
		return result, err
	}
	// The source records an actual capture failure within this transaction.
	// Financial evidence and the shared emergency stop remain durable together.
	var observation int
	if observer := r.h.localOperations(); observer != nil {
		emitEvent := c.Kind == "unpriced" || breach && d.MaximumBreached
		observation, err = observer.CaptureReconciliation(ctx, tx, r.h.cfg.Namespace, c.ID, emitEvent)
		if err != nil {
			return result, err
		}
		if err = r.authorize(ctx, tx, p); err != nil {
			return result, err
		}
	}
	if err = tx.Commit(); err != nil {
		return result, ErrUnavailable
	}
	if observation == 2 {
		_ = r.h.observeAuthorityFault(ctx, accountstore.OperationsAuthorityFault, ErrUnavailable)
	} else if observation == 1 {
		_ = r.h.deliverOperations(ctx)
	}
	if err = r.h.fault("after_reconciliation_commit"); err != nil {
		return result, err
	}
	return result, nil
}
func (r *Reconciliation) Status(ctx context.Context, p auth.Principal, id string) (ReconciliationStatus, error) {
	tx, d, v, err := r.begin(ctx, p, id, false)
	if err != nil {
		return ReconciliationStatus{}, err
	}
	defer tx.Rollback()
	out := ReconciliationStatus{Dispatch: d.ID, Counted: d.Counted, Maximum: d.Maximum, Remote: d.Remote, Category: d.Category, Compacted: d.Compacted, Version: v, Expense: d.Expense, Floor: d.Floor}
	if tx.QueryRowContext(ctx, `SELECT next_review,unpriced,attribution_floor FROM ai_recon_state WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, id).Scan(&out.NextReview, &out.Unpriced, &out.AttributionFloor) != nil {
		return out, ErrUnavailable
	}
	if tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT stopped FROM ai_recon_stop WHERE namespace=$1),false)`, r.h.cfg.Namespace).Scan(&out.AdmissionStopped) != nil {
		return out, ErrUnavailable
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.scope,s.counted,s.occupied FROM ai_fixture_scope s JOIN ai_fixture_allocation a ON a.namespace=s.namespace AND a.scope=s.scope WHERE a.namespace=$1 AND a.dispatch=$2 ORDER BY s.scope`, r.h.cfg.Namespace, id)
	if err != nil {
		return out, ErrUnavailable
	}
	for rows.Next() {
		var scope ReconciliationScope
		if rows.Scan(&scope.Name, &scope.Counted, &scope.Occupied) != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		out.Scopes = append(out.Scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, ErrUnavailable
	}
	return out, r.authorize(ctx, tx, p)
}

func decodeDigest(s string) []byte { raw, _ := hex.DecodeString(s); return raw }

func (r *Reconciliation) evidenceDigest(c ReconciliationCommand) string {
	return r.digest("evidence", "immutable-source-revision", struct {
		Dispatch, Kind                           string
		Evidence                                 ReconciliationEvidence
		Retention                                *DetailRetirementReview
		StopVersion                              int64
		BoundsReference, ReconciliationReference string
		RateRevision                             *RecordedRateRevision
	}{c.Dispatch, c.Kind, c.Evidence, c.Retention, c.StopVersion, c.BoundsReference, c.ReconciliationReference, c.RateRevision})
}
func (r *Reconciliation) priorEvidence(ctx context.Context, tx *sql.Tx, c ReconciliationCommand) (ReconciliationPreview, bool, error) {
	var binding, raw []byte
	var out ReconciliationPreview
	err := tx.QueryRowContext(ctx, `SELECT evidence_binding,result FROM ai_recon_command WHERE namespace=$1 AND dispatch=$2 AND source=$3 AND source_revision=$4`, r.h.cfg.Namespace, c.Dispatch, c.Evidence.Source, c.Evidence.Revision).Scan(&binding, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, ErrUnavailable
	}
	if !hmac.Equal(binding, decodeDigest(r.evidenceDigest(c))) {
		return out, true, ErrConflict
	}
	if json.Unmarshal(raw, &out) != nil {
		return out, true, ErrUnavailable
	}
	return out, true, nil
}

type ReconciliationEvent struct {
	ID                  string `json:"id"`
	Dispatch            string `json:"dispatch"`
	Contract            string `json:"contract"`
	Obligation          int64  `json:"obligation"`
	Reserved            int64  `json:"reserved"`
	OwnerReviewRequired bool   `json:"ownerReviewRequired"`
}

func (r *Reconciliation) Events(ctx context.Context, p auth.Principal) ([]ReconciliationEvent, error) {
	if r == nil || r.h == nil {
		return nil, ErrDenied
	}
	tx, err := r.h.reviewerDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback()
	if err = r.authorize(ctx, tx, p); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,dispatch,contract,obligation,reserved,owner_review_required FROM ai_recon_event WHERE namespace=$1 ORDER BY created_at,id LIMIT 100`, r.h.cfg.Namespace)
	if err != nil {
		return nil, ErrUnavailable
	}
	out := []ReconciliationEvent{}
	for rows.Next() {
		var event ReconciliationEvent
		if rows.Scan(&event.ID, &event.Dispatch, &event.Contract, &event.Obligation, &event.Reserved, &event.OwnerReviewRequired) != nil {
			rows.Close()
			return nil, ErrUnavailable
		}
		out = append(out, event)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrUnavailable
	}
	return out, r.authorize(ctx, tx, p)
}

func (r *Reconciliation) previewActor(ctx context.Context, tx *sql.Tx, p auth.Principal) (string, error) {
	if r.observer {
		return "runtime_observer/" + p.AccountID, nil
	}
	version := int64(0)
	if !r.runtime {
		if tx.QueryRowContext(ctx, `SELECT version FROM ai_recon_operator WHERE namespace=$1 AND actor=$2`, r.h.cfg.Namespace, p.AccountID).Scan(&version) != nil {
			return "", ErrDenied
		}
	}
	return fmt.Sprintf("%s/%s/%d/%d", p.AccountID, p.SessionID, p.Generation, version), nil
}

func financialEvidence(kind string) bool {
	return kind == "usage" || kind == "maximum" || kind == "bill" || kind == "corrected_bill"
}

// Signal hands the durable restricted events to an explicitly injected local
// sink. Delivery failure never changes containment. No network/notification
// driver is installed; sinks deduplicate by the immutable event ID.
func (r *Reconciliation) Signal(ctx context.Context, p auth.Principal, sink func(ReconciliationEvent) error) error {
	if sink == nil {
		return ErrDenied
	}
	events, err := r.Events(ctx, p)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := sink(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciliation) auditActor(p auth.Principal) string {
	if r.observer {
		return "runtime_observer"
	}
	return p.AccountID
}

package aicontrol

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestReconciliationRetainedAttributionAllowsExactReviewedCorrection(t *testing.T) {
	for _, usageNeeded := range []bool{true, false} {
		name := "all_fields_needed"
		if !usageNeeded {
			name = "bill_does_not_need_usage"
		}
		t.Run(name, func(t *testing.T) {
			f := setup(t, nil)
			s := f.open(t, "retained_adjustment")
			d := f.sent(t, s)
			operator := reconciliationOperator(t, f)
			r := f.h.Reconciliation()
			bill := reconciliationUsage(d, "retained_bill")
			bill.Kind = "bill"
			bill.Evidence.Usage = nil
			bill.Evidence.Gross = pointer(15)
			bill.Evidence.Difference = pointer(0)
			applyReconciliation(t, r, operator, bill)
			f.clock.set(d.IntentAt.Add(30*24*time.Hour), true)
			next := d.IntentAt.Add(37 * 24 * time.Hour)
			review := reconciliationUsage(d, "attribution_review")
			review.Kind = "compact"
			review.ExpectedVersion = 1
			review.Evidence.Usage = nil
			review.Evidence.Difference = nil
			review.Retention = &DetailRetirementReview{Reference: "pending_exact_bill_adjustment", Needed: []DetailPurpose{{Field: "request_id", Purpose: "adjustment"}, {Field: "source_reference", Purpose: "adjustment"}}, NextReview: &next}
			if usageNeeded {
				review.Retention.Needed = append(review.Retention.Needed, DetailPurpose{Field: "usage", Purpose: "adjustment"})
			}
			applyReconciliation(t, r, operator, review)
			state, err := r.Status(context.Background(), operator, d.ID)
			if err != nil || state.Counted != 15 || state.Remote != "possibly_active" {
				t.Fatal(state, err)
			}
			if usageNeeded && state.Compacted {
				t.Fatal("a full detail deferral was mislabeled retirement", state)
			}
			correction := bill
			correction.ID = "exact_correction"
			correction.Kind = "corrected_bill"
			correction.ExpectedVersion = 2
			correction.Evidence.Revision = 2
			correction.Evidence.Supersedes = "retained_bill"
			correction.Evidence.Gross = pointer(10)
			wrong := correction
			wrong.ID = "different_obligation"
			wrong.Evidence.RequestID = "other_request"
			if _, err := r.Preview(context.Background(), operator, wrong); err == nil {
				t.Fatal("retained data authorized another request")
			}
			result := applyReconciliation(t, r, operator, correction)
			recovered, err := r.Apply(context.Background(), operator, correction)
			if err != nil || !reflect.DeepEqual(result, recovered) {
				t.Fatal(result, recovered, err)
			}
			correction.Evidence.Gross = pointer(9)
			if _, err = r.Apply(context.Background(), operator, correction); !errors.Is(err, ErrConflict) {
				t.Fatal("changed corrected-bill replay accepted", err)
			}
			state, err = r.Status(context.Background(), operator, d.ID)
			if err != nil || state.Counted != 10 || state.Remote != "possibly_active" {
				t.Fatal(state, err)
			}
			for _, scope := range state.Scopes {
				if scope.Counted != 10 || scope.Occupied != 1 {
					t.Fatal(scope)
				}
			}
		})
	}
}

func TestReconciliationRetiredAttributionCannotBeRestoredByAReviewOrHigherBill(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "lost_attribution")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	bill := reconciliationUsage(d, "original_bill")
	bill.Kind = "bill"
	bill.Evidence.Usage = nil
	bill.Evidence.Gross = pointer(15)
	bill.Evidence.Difference = pointer(0)
	applyReconciliation(t, r, operator, bill)
	f.clock.set(d.IntentAt.Add(30*24*time.Hour), true)
	retire := reconciliationUsage(d, "actual_retirement")
	retire.Kind = "compact"
	retire.ExpectedVersion = 1
	retire.Evidence.Usage = nil
	retire.Evidence.Difference = nil
	retire.Retention = &DetailRetirementReview{Reference: "no_detail_purpose"}
	applyReconciliation(t, r, operator, retire)
	next := d.IntentAt.Add(37 * 24 * time.Hour)
	deferAgain := retire
	deferAgain.ID = "later_review"
	deferAgain.ExpectedVersion = 2
	deferAgain.Evidence.Source = "later_review"
	deferAgain.Retention = &DetailRetirementReview{Reference: "new_adjustment_purpose", Needed: []DetailPurpose{{Field: "usage", Purpose: "adjustment"}, {Field: "request_id", Purpose: "adjustment"}, {Field: "source_reference", Purpose: "adjustment"}}, NextReview: &next}
	applyReconciliation(t, r, operator, deferAgain)
	lower := bill
	lower.ID = "unprovable_lower"
	lower.Kind = "corrected_bill"
	lower.ExpectedVersion = 3
	lower.Evidence.Revision = 2
	lower.Evidence.Supersedes = "original_bill"
	lower.Evidence.Gross = pointer(10)
	if _, err := r.Preview(context.Background(), operator, lower); err == nil {
		t.Fatal("review resurrected retired attribution")
	}
	higher := bill
	higher.ID = "fresh_higher_bill"
	higher.ExpectedVersion = 3
	higher.Evidence.Source = "fresh_higher_source"
	higher.Evidence.Gross = pointer(90)
	applyReconciliation(t, r, operator, higher)
	lower.ID = "lower_after_higher"
	lower.ExpectedVersion = 4
	lower.Evidence.Source = "fresh_higher_source"
	lower.Evidence.Revision = 2
	lower.Evidence.Supersedes = "fresh_higher_bill"
	if _, err := r.Preview(context.Background(), operator, lower); err == nil {
		t.Fatal("higher evidence erased old unverifiable floor")
	}
	state, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || state.Counted != 90 || state.Remote != "possibly_active" || !state.AdmissionStopped {
		t.Fatal(state, err)
	}
}

func TestReconciliationPriorSchemaUpgradeDistinguishesDeferralFromLoss(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "retained"
		if lost {
			name = "retired"
		}
		t.Run(name, func(t *testing.T) {
			f := setup(t, nil)
			s := f.open(t, "old_schema")
			d := f.sent(t, s)
			operator := reconciliationOperator(t, f)
			r := f.h.Reconciliation()
			bill := reconciliationUsage(d, "old_schema_bill")
			bill.Kind = "bill"
			bill.Evidence.Usage = nil
			bill.Evidence.Gross = pointer(15)
			bill.Evidence.Difference = pointer(0)
			applyReconciliation(t, r, operator, bill)
			f.clock.set(d.IntentAt.Add(30*24*time.Hour), true)
			review := reconciliationUsage(d, "old_schema_review")
			review.Kind = "compact"
			review.ExpectedVersion = 1
			review.Evidence.Usage = nil
			review.Evidence.Difference = nil
			review.Retention = &DetailRetirementReview{Reference: "original_review"}
			if !lost {
				next := d.IntentAt.Add(37 * 24 * time.Hour)
				review.Retention.Needed = []DetailPurpose{{Field: "usage", Purpose: "adjustment"}, {Field: "request_id", Purpose: "adjustment"}, {Field: "source_reference", Purpose: "adjustment"}}
				review.Retention.NextReview = &next
			}
			applyReconciliation(t, r, operator, review)
			// Exact old schema/flag shape: every explicit review set compacted=true,
			// and no attribution_floor column existed. Only this disposable migrator
			// can alter the fixture; runtime/operator credentials cannot do so.
			if _, err := f.db.Exec(`UPDATE ai_fixture_dispatch SET compacted=true WHERE id=$1`, d.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`ALTER TABLE ai_recon_state DROP COLUMN attribution_floor`); err != nil {
				t.Fatal(err)
			}
			upgraded, err := NewSynthetic(context.Background(), f.dsn, f.cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			correction := bill
			correction.Kind = "corrected_bill"
			correction.ID = "upgraded_correction"
			correction.ExpectedVersion = 2
			correction.Evidence.Revision = 2
			correction.Evidence.Supersedes = "old_schema_bill"
			correction.Evidence.Gross = pointer(10)
			if lost {
				if _, err = upgraded.Reconciliation().Preview(context.Background(), operator, correction); err == nil {
					t.Fatal("migration forgot actual attribution loss")
				}
				state, err := upgraded.Reconciliation().Status(context.Background(), operator, d.ID)
				if err != nil || state.Counted != 15 || state.AttributionFloor != 15 {
					t.Fatal(state, err)
				}
			} else {
				applyReconciliation(t, upgraded.Reconciliation(), operator, correction)
				state, err := upgraded.Reconciliation().Status(context.Background(), operator, d.ID)
				if err != nil || state.Counted != 10 || state.AttributionFloor != 0 {
					t.Fatal(state, err)
				}
			}
		})
	}
}

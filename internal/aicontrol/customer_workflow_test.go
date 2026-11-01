package aicontrol

import (
	"context"
	"errors"
	"testing"
)

func TestA22_DraftAllowanceBelongsOnlyToApplicationDraft(t *testing.T) {
	for _, workflow := range []string{"cv_generation", "profile_review", "qualification_gaps", "profile_ingestion"} {
		t.Run(workflow, func(t *testing.T) {
			f := setup(t, nil)
			// Qualification analysis legitimately requires the saved Profile but
			// does not inherit the separate customer Application Draft allowance.
			saveLifecycleProfile(t, f)
			s, err := f.h.Open(context.Background(), f.p, "not_a_draft", workflow, []byte(`{"synthetic":"root"}`))
			if err != nil {
				t.Fatal(err)
			}
			if err := s.ReserveCustomer(context.Background()); !errors.Is(err, ErrDenied) {
				t.Fatalf("non-Draft reservation: %v", err)
			}
			r := f.h.SyntheticReviewer()
			for _, kind := range []string{"candidate", "open"} {
				err := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: kind, ResultRef: "synthetic_result", Complete: true, Recoverable: true})
				if !errors.Is(err, ErrDenied) {
					t.Fatalf("non-Draft %s: %v", kind, err)
				}
			}
			status := f.status(t, s)
			if status.CustomerReserved || status.CustomerEffect || status.State != "recoverable" || availableCustomerUnits(t, f) != 3 || f.calls.Load() != 0 {
				t.Fatalf("non-Draft workflow changed allowance/delivery: %+v", status)
			}
		})
	}

	t.Run("application_draft", func(t *testing.T) {
		f := setup(t, nil)
		s := f.openDraft(t, "real_draft_fixture")
		if err := s.ReserveCustomer(context.Background()); err != nil {
			t.Fatal(err)
		}
		r := f.h.SyntheticReviewer()
		for _, kind := range []string{"candidate", "open", "open", "accept", "export"} {
			if err := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: kind, ResultRef: "synthetic_complete_result", Complete: true, Recoverable: true}); err != nil {
				t.Fatal(kind, err)
			}
		}
		status := f.status(t, s)
		if !status.CustomerEffect || status.CustomerReserved || status.State != "delivered" || availableCustomerUnits(t, f) != 2 || f.calls.Load() != 0 {
			t.Fatalf("Draft did not receive one synthetic allowance effect: %+v", status)
		}
	})
}

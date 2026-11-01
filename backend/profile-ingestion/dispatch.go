package profileingestion

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"professional-information-repo/internal/aidispatch"
)

// dispatchCoordinate separates the existing server-validated protocol branches.
// Handoff verification remains mandatory before a provider call. Coordinates
// are restricted replay linkage, never ordinary diagnostic fields or authority.
func dispatchCoordinate(in request) string {
	coordinate := "ingest/legacy"
	if in.Stage != nil {
		coordinate = "ingest/v" + strconv.Itoa(in.Stage.Version) + "/" + in.Stage.Kind
	}
	if in.Portion != nil {
		coordinate += "/portion/" + strconv.Itoa(in.Portion.Index)
	}
	if in.BatchIndex != nil {
		coordinate += "/batch/" + strconv.Itoa(*in.BatchIndex)
	}
	if in.Focus != nil {
		coordinate += "/focus/" + canonicalBinding(in.Focus)
	}
	if in.Claim != nil {
		coordinate += "/claim/" + canonicalBinding(in.Claim.ID)
	}
	if in.Clarification != nil {
		coordinate += "/clarification/" + canonicalBinding(in.Clarification.Claim.ID)
	}
	return coordinate
}

func gateCoordinate(ctx context.Context, leg string) context.Context {
	return aidispatch.WithCoordinate(ctx, aidispatch.Coordinate(ctx)+"/"+leg)
}

func bindProviderFreeStage(ctx context.Context, fail func(int, string)) (aidispatch.StageReceipt, bool) {
	receipt, err := aidispatch.BindStage(ctx, aidispatch.Coordinate(ctx), aidispatch.StageInput(ctx))
	if err == nil {
		return receipt, true
	}
	if errors.Is(err, aidispatch.ErrBindingConflict) {
		fail(http.StatusConflict, "stage_binding")
	} else {
		fail(http.StatusServiceUnavailable, "admission_unavailable")
	}
	return aidispatch.StageReceipt{}, false
}

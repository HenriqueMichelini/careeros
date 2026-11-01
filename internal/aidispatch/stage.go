package aidispatch

import (
	"context"
	"errors"
	"time"
)

var ErrBindingConflict = errors.New("immutable AI stage binding conflict")

// StageReceipt contains only the stable identity and clock of a provider-free
// stage. It makes repeated signed handoffs deterministic without retaining their
// career-content preimage or inventing a provider dispatch.
type StageReceipt struct {
	ID        string
	CreatedAt time.Time
}

type stageBinder interface {
	BindStage(context.Context, string, []byte) (StageReceipt, error)
}

type clarificationRevision interface {
	ClarificationRevision() string
}

// ClarificationRevision is supplied only by the code-installed authority after
// an explicit bounded child-creation command. HTTP input cannot select it.
func ClarificationRevision(ctx context.Context) string {
	controller, _ := ctx.Value(controllerKey).(Controller)
	child, ok := controller.(clarificationRevision)
	if !ok {
		return ""
	}
	return child.ClarificationRevision()
}

// BindStage is a no-op for the existing explicit BYOK path. Managed execution
// requires the same installed authority even when a protocol stage sends no AI.
func BindStage(ctx context.Context, coordinate string, input []byte) (StageReceipt, error) {
	managed, _ := ctx.Value(managedKey).(bool)
	if !managed {
		return StageReceipt{}, nil
	}
	controller, _ := ctx.Value(controllerKey).(Controller)
	binder, ok := controller.(stageBinder)
	if !ok {
		return StageReceipt{}, ErrUnavailable
	}
	return binder.BindStage(ctx, coordinate, input)
}

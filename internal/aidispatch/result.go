package aidispatch

import "context"

// ResultAuthority is an additional protected-result barrier for a managed
// controller. Admission permission alone cannot authorize a later result release.
type ResultAuthority interface{ ResultAllowed(context.Context) error }

// ResultScope keeps an already-authorized HTTP write visible to recovery until
// that write returns. Cancellation cannot recall bytes already handed to a
// ResponseWriter; a blocked writer must prevent a successful drain instead.
type ResultScope interface {
	BeginResult(context.Context) (context.Context, func(), error)
}

func WriteResult(ctx context.Context, write func(context.Context) error) error {
	managed, _ := ctx.Value(managedKey).(bool)
	if !managed {
		return write(ctx)
	}
	controller, _ := ctx.Value(controllerKey).(Controller)
	authority, ok := controller.(ResultScope)
	if !ok || authority == nil {
		return ErrUnavailable
	}
	resultCtx, finish, err := authority.BeginResult(ctx)
	if err != nil {
		return err
	}
	defer finish()
	if resultCtx.Err() != nil {
		return resultCtx.Err()
	}
	if err := write(resultCtx); err != nil {
		return err
	}
	return resultCtx.Err()
}

func CheckResult(ctx context.Context) error {
	managed, _ := ctx.Value(managedKey).(bool)
	if !managed {
		return nil
	}
	controller, _ := ctx.Value(controllerKey).(Controller)
	authority, ok := controller.(ResultAuthority)
	if !ok || authority == nil {
		return ErrUnavailable
	}
	return authority.ResultAllowed(ctx)
}

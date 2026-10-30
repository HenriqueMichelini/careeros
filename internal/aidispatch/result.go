package aidispatch

import "context"

// ResultAuthority is an additional protected-result barrier for a managed
// controller. Admission permission alone cannot authorize a later result release.
type ResultAuthority interface{ ResultAllowed(context.Context) error }

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

package aicontrol

// ActionReference exposes the server-owned identity for restricted-purpose
// result repositories. It is not dispatch, grant, delivery or allowance authority.
type ActionReference struct{ Namespace, ID, Owner, Workflow string }

func (s *Session) ActionReference() ActionReference {
	if s == nil || s.h == nil {
		return ActionReference{}
	}
	return ActionReference{s.h.cfg.Namespace, s.id, s.p.AccountID, s.workflow}
}

// Namespace identifies the configured shared authority, without granting access.
func (h *Harness) Namespace() string {
	if h == nil {
		return ""
	}
	return h.cfg.Namespace
}

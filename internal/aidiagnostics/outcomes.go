package aidiagnostics

// OutcomeNormalizerVersion identifies a derivation, never a rewrite of saved data.
const OutcomeNormalizerVersion = "terminal-outcomes-v1"

// OperationalOutcome classifies the frozen terminal vocabulary. Unknown values
// deliberately stay unknown; HTTP or assertion success cannot upgrade them.
func OperationalOutcome(outcome string) string {
	switch outcome {
	case "completed", "complete":
		return "completed"
	case "request_information":
		return "requested_information"
	case "input_rejected", "reject", "irrelevant", "rephrase", "refused":
		return "policy_rejection"
	case "incomplete", "incomplete_output", "truncated":
		return "incomplete"
	case "canceled", "cancelled":
		return "canceled"
	case "unavailable", "assessment_unavailable":
		return "assessment_unavailable"
	case "transport_failure", "provider_failure", "timeout", "outage":
		return "infrastructure_failure"
	case "malformed_output", "rejected_output", "invalid_output":
		return "invalid_output"
	default:
		return "unknown"
	}
}

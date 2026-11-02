package aicontrol

// QualificationRateID identifies the owner-selected public reference rates of
// 2026-11-02. Amounts in this mode are integer microUSD, never confirmed bills.
const QualificationRateID = "qualification-usd-20261102-v1"
const QualificationAttemptMaximum int64 = 25344

// Every numeric operating input is frozen into the Action execution binding.
// A later binary cannot reinterpret an admitted Action by retaining a label
// while changing its limits or public-reference rates.
type qualificationEnvelope struct {
	Actions, RollingActions, Attempts, TypeSafeCalls, OpenAICalls               int
	TypeSafeInput, OpenAIInput, OpenAIOutput, AttemptOutput                     int64
	AttemptMoney, ActionMoney, AccountDay, AccountTotal, GlobalDay, GlobalTotal int64
	AccountActive, GlobalActive, AccountOutstanding, GlobalOutstanding          int
	TypeSafeRate, OrdinaryRate, CachedRate, CacheWriteRate, OutputRate          int64
}

func selectedQualificationEnvelope() qualificationEnvelope {
	return qualificationEnvelope{
		Actions: 20, RollingActions: 5, Attempts: 2, TypeSafeCalls: 1, OpenAICalls: 2,
		TypeSafeInput: 32000, OpenAIInput: 64000, OpenAIOutput: 8000, AttemptOutput: 16000,
		AttemptMoney: 30000, ActionMoney: 60000, AccountDay: 300000, AccountTotal: 2000000, GlobalDay: 1500000, GlobalTotal: 10000000,
		AccountActive: 1, GlobalActive: 2, AccountOutstanding: 1, GlobalOutstanding: 2,
		TypeSafeRate: 42, OrdinaryRate: 100, CachedRate: 10, CacheWriteRate: 125, OutputRate: 500,
	}
}

// QualificationEstimate prices complete, explicitly compatible usage. Cached
// input and cache writes partition total input; reasoning is already in output.
// Explicit zero categories are required for OpenAI: absence is not free usage.
func QualificationEstimate(provider, model, tier, rate string, u Usage) (int64, error) {
	if rate != QualificationRateID || tier != "standard" || u.Unsupported || u.Input == nil || u.Output == nil {
		return 0, ErrDenied
	}
	for _, n := range []*int64{u.Input, u.Output, u.Cached, u.CacheWrite, u.Reasoning} {
		if n != nil && (*n < 0 || *n > 1<<40) {
			return 0, ErrDenied
		}
	}
	var numerator int64
	p := selectedQualificationEnvelope()
	switch provider {
	case "typesafe":
		if model != "jev-1.13.0" || u.Cached != nil || u.CacheWrite != nil || u.Reasoning != nil {
			return 0, ErrDenied
		}
		numerator = *u.Input * p.TypeSafeRate // 0.042 USD per million, in 1/1000 microUSD.
	case "openai":
		if model != "gpt-6-luna" || u.Cached == nil || u.CacheWrite == nil || u.Reasoning == nil || *u.Cached+*u.CacheWrite > *u.Input || *u.Reasoning > *u.Output {
			return 0, ErrDenied
		}
		numerator = (*u.Input-*u.Cached-*u.CacheWrite)*p.OrdinaryRate + *u.Cached*p.CachedRate + *u.CacheWrite*p.CacheWriteRate + *u.Output*p.OutputRate
	default:
		return 0, ErrDenied
	}
	return (numerator + 999) / 1000, nil
}

package aicontrol

// IngestionAttemptMaximum is the owner-selected cumulative input/output
// reference-rate arithmetic, before upward rounding of individual reservations.
// It is not a flat charge, upfront reservation or confirmed provider bill.
const IngestionAttemptMaximum int64 = 297012
const IngestionPolicyVersion = "ingestion-local-20261103-v1"

type ingestionEnvelope struct {
	Actions, RollingActions, Attempts, Extract, Reconcile, Clarify, TypeSafeCalls, OpenAICalls int
	TypeSafeInput, TypeSafeTotal, OpenAIInput, OpenAITotal, OpenAIOutput, OutputTotal          int64
	AttemptMoney, ActionMoney                                                                  int64
}

func selectedIngestionEnvelope() ingestionEnvelope {
	return ingestionEnvelope{10, 3, 2, 20, 20, 4, 48, 48, 32000, 1536000, 64000, 900000, 6000, 240000, 300000, 600000}
}

// IngestionReservation uses the complete synthetic request's trustworthy upper
// input bound and full maximum output, with no assumed cache savings. The rates
// are the frozen 2 November public-reference snapshot shared with qualification.
func IngestionReservation(provider string, input, output int64) (int64, error) {
	p := selectedIngestionEnvelope()
	if input < 0 || output < 0 {
		return 0, ErrDenied
	}
	var numerator int64
	switch provider {
	case "typesafe":
		if input > p.TypeSafeInput || output != 0 {
			return 0, ErrCapacity
		}
		numerator = input * selectedQualificationEnvelope().TypeSafeRate
	case "openai":
		if input > p.OpenAIInput || output <= 0 || output > p.OpenAIOutput {
			return 0, ErrCapacity
		}
		numerator = input*selectedQualificationEnvelope().CacheWriteRate + output*selectedQualificationEnvelope().OutputRate
	default:
		return 0, ErrDenied
	}
	return (numerator + 999) / 1000, nil
}

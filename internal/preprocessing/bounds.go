package preprocessing

import (
	"errors"

	"professional-information-repo/internal/fieldvalidation"
)

type CapacityStatus string

const (
	Ready                CapacityStatus = "ready"
	InsufficientCapacity CapacityStatus = "insufficient_capacity"
)

// FieldLimits retains existing decoded-source and encoded HTTP request limits.
// Callers must check transport/schema and the actual request body before decoding.
type FieldLimits struct{ OriginalBytes, RequestBodyBytes int }

func LimitsFor(field fieldvalidation.Field) (FieldLimits, error) {
	switch field {
	case fieldvalidation.ProfessionalInformation:
		return FieldLimits{30000, 160 << 10}, nil
	case fieldvalidation.JobPosting:
		return FieldLimits{30 << 10, 128 << 10}, nil
	default:
		return FieldLimits{}, errors.New("unknown source field")
	}
}

// CheckRequestBodySize checks actual encoded transport bytes, including JSON
// escaping and envelope fields. It must run before decoding/normalization;
// decoded source and provider payload limits are separate checks.
func CheckRequestBodySize(field fieldvalidation.Field, size int) (CapacityStatus, error) {
	limits, err := LimitsFor(field)
	if err != nil {
		return "", err
	}
	if size < 0 {
		return "", errors.New("negative request body size")
	}
	if size > limits.RequestBodyBytes {
		return InsufficientCapacity, nil
	}
	return Ready, nil
}

// Preparation retains the whole original even when raw capacity is insufficient.
// An oversize input is never normalized; PreparedBytes then remains zero.
// Ready describes byte capacity only, never whole-source Jev acceptance.
type Preparation struct {
	Status                       CapacityStatus
	Original                     string
	OriginalBytes, PreparedBytes int
	Source                       Source
	Unprocessed                  []Range
}

func PrepareBounded(original string, field fieldvalidation.Field) (Preparation, error) {
	limits, err := LimitsFor(field)
	if err != nil {
		return Preparation{}, err
	}
	r := Preparation{Original: original, OriginalBytes: len(original)}
	if len(original) > limits.OriginalBytes {
		r.Status = InsufficientCapacity
		r.Unprocessed = []Range{{0, len(original)}}
		return r, nil
	}
	r.Source, err = Prepare(original, field)
	if err != nil {
		return r, err
	}
	r.PreparedBytes = len(r.Source.Text())
	r.Status = Ready
	return r, nil
}

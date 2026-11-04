package aicontrol_test

import (
	"testing"

	"professional-information-repo/internal/aicontrol"
)

func TestManagedIngestionSelectedExposureBounds(t *testing.T) {
	// Independent integer arithmetic from the approved cumulative quantities
	// and frozen microUSD-per-thousand-token rates, not per-call assumptions.
	const numerator = int64(1536000*42 + 900000*125 + 240000*500)
	if numerator%1000 != 0 || numerator/1000 != 297012 || aicontrol.IngestionAttemptMaximum != numerator/1000 {
		t.Fatal("selected aggregate maximum must remain 297,012 microUSD")
	}
	for _, tc := range []struct {
		provider            string
		input, output, want int64
	}{
		{"typesafe", 32000, 0, 1344},
		{"openai", 64000, 6000, 11000},
		{"typesafe", 1, 0, 1},
		{"openai", 1, 6000, 3001},
	} {
		n, err := aicontrol.IngestionReservation(tc.provider, tc.input, tc.output)
		if err != nil || n != tc.want {
			t.Fatalf("%s %d/%d = %d %v; want %d", tc.provider, tc.input, tc.output, n, err, tc.want)
		}
	}
	for _, tc := range []struct {
		provider      string
		input, output int64
	}{
		{"typesafe", 32001, 0}, {"openai", 64001, 6000}, {"openai", 64000, 6001}, {"openai", -1, 6000},
	} {
		if _, err := aicontrol.IngestionReservation(tc.provider, tc.input, tc.output); err == nil {
			t.Fatalf("accepted incompatible %v", tc)
		}
	}
}

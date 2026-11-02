package aicontrol_test

import (
	"professional-information-repo/internal/aicontrol"
	"testing"
)

func TestQualificationSelectedRateArithmetic(t *testing.T) {
	p := func(n int64) *int64 { return &n }
	cases := []struct {
		name, provider string
		usage          aicontrol.Usage
		want           int64
	}{
		{"typesafe free output", "typesafe", aicontrol.Usage{Input: p(32000), Output: p(5000)}, 1344},
		{"maximum cache writes", "openai", aicontrol.Usage{Input: p(64000), Output: p(8000), Cached: p(0), CacheWrite: p(64000), Reasoning: p(0)}, 12000},
		{"subsets charged once", "openai", aicontrol.Usage{Input: p(10000), Output: p(2000), Cached: p(4000), CacheWrite: p(1000), Reasoning: p(500)}, 1665},
		{"fraction rounds upward", "typesafe", aicontrol.Usage{Input: p(1), Output: p(0)}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := "gpt-6-luna"
			if tc.provider == "typesafe" {
				model = "jev-1.13.0"
			}
			got, err := aicontrol.QualificationEstimate(tc.provider, model, "standard", aicontrol.QualificationRateID, tc.usage)
			if err != nil || got != tc.want {
				t.Fatalf("microUSD = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
	if aicontrol.QualificationAttemptMaximum != 25344 {
		t.Fatal("independently expected US$0.025344 maximum")
	}
	for _, u := range []aicontrol.Usage{
		{Input: p(10), Output: p(2)},
		{Input: p(10), Output: p(2), Cached: p(9), CacheWrite: p(2), Reasoning: p(0)},
		{Input: p(10), Output: p(2), Cached: p(0), CacheWrite: p(0), Reasoning: p(3)},
	} {
		if _, err := aicontrol.QualificationEstimate("openai", "gpt-6-luna", "standard", aicontrol.QualificationRateID, u); err == nil {
			t.Fatal("partial/incompatible usage priced")
		}
	}
}

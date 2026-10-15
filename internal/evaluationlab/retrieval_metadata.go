package evaluationlab

var retrievalNegativeIDs = map[string][]string{
	"explicit-support":                 {},
	"composite-partial":                {},
	"missing-not-lacking":              {},
	"aspiration":                       {"wish"},
	"negation":                         {"no"},
	"approximate-duration":             {},
	"related-technology":               {},
	"technology-similarity":            {},
	"contrasting-employers":            {},
	"project-not-employment":           {},
	"portuguese-negative":              {"no"},
	"material-miss-under-budget":       {},
	"incomplete-cannot-assert-absence": {},
}
var retrievalLanguages = map[string]string{
	"explicit-support":                 "en",
	"composite-partial":                "en",
	"missing-not-lacking":              "en",
	"aspiration":                       "en",
	"negation":                         "en",
	"approximate-duration":             "en",
	"related-technology":               "en",
	"technology-similarity":            "en",
	"contrasting-employers":            "en",
	"project-not-employment":           "en",
	"portuguese-negative":              "pt-BR",
	"material-miss-under-budget":       "en",
	"incomplete-cannot-assert-absence": "en",
}

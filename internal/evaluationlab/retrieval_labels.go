package evaluationlab

// Unique reviewed IDs in retained synthetic retrieval labels.
const retrievalLabelHash = "6caa5ddf5077785da0a3865a23b9956ccb72080748e688c6b15479c9b1be2684"

var retrievalRelevantIDs = map[string][]string{
	"explicit-support":                 {"java"},
	"composite-partial":                {"java"},
	"missing-not-lacking":              {},
	"aspiration":                       {"wish"},
	"negation":                         {"no"},
	"approximate-duration":             {"java"},
	"related-technology":               {"spring"},
	"technology-similarity":            {"js"},
	"contrasting-employers":            {"a-company", "a-java", "b-aws", "b-company"},
	"project-not-employment":           {"p-java", "p-name"},
	"portuguese-negative":              {"no"},
	"material-miss-under-budget":       {"java"},
	"incomplete-cannot-assert-absence": {},
}

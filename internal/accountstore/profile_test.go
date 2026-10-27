package accountstore

import "testing"

func TestStrictJSONPreservesNULAndRejectsReplacementEncodings(t *testing.T) {
	for _, value := range []string{`{"v":"ação\u0000\ud83d\ude00"}`, `{"v":"\\ud800"}`, `{"v":"quote: \" and slash: \\"}`, `{"v":["\ud800\udc00","\udbff\udfff",{"\ud83d\ude00":"text"}]}`, `{"v":[1e308,5e-324,9007199254740991]}`} {
		if !StrictJSON([]byte(value)) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{`{"v":"\ud800"}`, `{"v":"\udc00"}`, `{"v":"\ud800\ud800"}`, "{\"v\":\"\xff\"}", `{} {}`, `{"v":NaN}`, `{"a":1,"\u0061":2}`, `{"v":"\ud800x"}`, `{"v":"\ud800\n"}`, `{"v":"\ud800\u0061"}`, `{"v":["safe",{"\udfff":"bad key"}]}`} {
		if StrictJSON([]byte(value)) {
			t.Fatal(value)
		}
	}
}

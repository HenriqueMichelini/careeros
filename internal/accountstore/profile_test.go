package accountstore

import "testing"

func TestStrictJSONPreservesNULAndRejectsReplacementEncodings(t *testing.T) {
	for _, value := range []string{`{"v":"ação\u0000\ud83d\ude00"}`, `{"v":"\\ud800"}`, `{"v":[1e308,5e-324,9007199254740991]}`} {
		if !StrictJSON([]byte(value)) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{`{"v":"\ud800"}`, `{"v":"\udc00"}`, `{"v":"\ud800\ud800"}`, "{\"v\":\"\xff\"}", `{} {}`, `{"v":NaN}`, `{"a":1,"\u0061":2}`} {
		if StrictJSON([]byte(value)) {
			t.Fatal(value)
		}
	}
}

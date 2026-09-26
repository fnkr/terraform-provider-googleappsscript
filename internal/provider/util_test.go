package provider

import "testing"

func TestFileNames(t *testing.T) {
	for _, name := range []string{"appsscript.json", "Code.gs", "lib/util.gs", "Index.html"} {
		f, err := toAPIFile(name, "")
		if err != nil {
			t.Fatalf("toAPIFile(%q): %v", name, err)
		}
		got, err := fromAPIFile(f)
		if err != nil || got != name {
			t.Errorf("round trip %q: got %q, %v", name, got, err)
		}
	}
	for _, name := range []string{"Code.js", ".gs", "README"} {
		if _, err := toAPIFile(name, ""); err == nil {
			t.Errorf("toAPIFile(%q): expected error", name)
		}
	}
}

func TestJSONEqual(t *testing.T) {
	if !jsonEqual(`{"a":1,"b":[2]}`, "{\n  \"b\": [2],\n  \"a\": 1\n}") {
		t.Error("expected equal")
	}
	if jsonEqual(`{"a":1}`, `{"a":2}`) || jsonEqual(`{`, `{`) {
		t.Error("expected not equal")
	}
}

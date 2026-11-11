package httpapi

import "testing"

func TestRecentForgetsTheOldestEntryOnceFull(t *testing.T) {
	r := newRecent[string](2)
	r.put("a", "A")
	r.put("b", "B")
	r.put("a", "A2")
	r.put("c", "C")
	if _, ok := r.get("a"); ok {
		t.Error("kept the oldest entry")
	}
	for k, want := range map[string]string{"b": "B", "c": "C"} {
		if got, ok := r.get(k); !ok || got != want {
			t.Errorf("%s: got %q %v", k, got, ok)
		}
	}
}

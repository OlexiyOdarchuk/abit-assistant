package service

import (
	"strings"
	"testing"
)

func TestIsMaskedName(t *testing.T) {
	cases := map[string]bool{
		"Іва### О В": true,
		"Бовкун О В": false,
		"І":          false,
		"###":        true,
	}
	for name, want := range cases {
		if got := isMaskedName(name); got != want {
			t.Errorf("isMaskedName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestMaskName(t *testing.T) {
	a := maskName("Бовкун Олексій Володимирович")
	// Stable and non-reversible: same input → same tag, prefixed, no PII.
	if a != maskName("Бовкун Олексій Володимирович") {
		t.Error("maskName is not stable for the same input")
	}
	if !strings.HasPrefix(a, "name#") {
		t.Errorf("maskName tag %q missing name# prefix", a)
	}
	if strings.Contains(a, "Бовкун") {
		t.Errorf("maskName leaked the name: %q", a)
	}
	if maskName("A") == maskName("B") {
		t.Error("different names collided")
	}
}

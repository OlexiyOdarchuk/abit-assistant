package callback

import "testing"

func TestEncode(t *testing.T) {
	if got := Encode("page", "all", "3"); got != "page|all|3" {
		t.Errorf("Encode = %q", got)
	}
	if got := Encode(); got != "" {
		t.Errorf("Encode() = %q, want empty", got)
	}
}

func TestArgs_Accessors(t *testing.T) {
	a := FromString("competitors|3|42")
	if a.Len() != 3 {
		t.Errorf("Len = %d", a.Len())
	}
	if a.String(0) != "competitors" {
		t.Errorf("String(0) = %q", a.String(0))
	}
	if a.String(99) != "" {
		t.Errorf("String(99) should be empty")
	}
	if n, ok := a.Int(1); !ok || n != 3 {
		t.Errorf("Int(1) = (%d, %v)", n, ok)
	}
	if n, ok := a.Int64(2); !ok || n != 42 {
		t.Errorf("Int64(2) = (%d, %v)", n, ok)
	}
}

func TestArgs_ParseFailures(t *testing.T) {
	a := FromString("x|notanumber")
	if _, ok := a.Int(1); ok {
		t.Error("expected Int parse to fail")
	}
	if _, ok := a.Int(99); ok {
		t.Error("expected out-of-range to fail")
	}
}

func TestArgs_Zero(t *testing.T) {
	var a Args
	if a.Len() != 0 {
		t.Errorf("zero Len = %d", a.Len())
	}
	if a.String(0) != "" {
		t.Error("zero String should be empty")
	}
	if _, ok := a.Int(0); ok {
		t.Error("zero Int should fail")
	}
}

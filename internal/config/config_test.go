package config

import (
	"testing"
)

func TestParseInt64List(t *testing.T) {
	tests := []struct {
		in      string
		want    []int64
		wantErr bool
	}{
		{"", nil, false},
		{"123", []int64{123}, false},
		{"123, 456, 789", []int64{123, 456, 789}, false},
		{"  123 ,  456  ", []int64{123, 456}, false},
		{"123,,456", []int64{123, 456}, false},
		{"abc", nil, true},
		{"123, abc", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseInt64List(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalInt64Slices(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAdmin(t *testing.T) {
	c := &Config{AdminIDs: []int64{42, 100}}
	if !c.IsAdmin(42) {
		t.Error("42 should be admin")
	}
	if c.IsAdmin(999) {
		t.Error("999 should not be admin")
	}
	empty := &Config{}
	if empty.IsAdmin(42) {
		t.Error("no admins: 42 should not be admin")
	}
}

func TestValidate(t *testing.T) {
	if err := (&Config{}).Validate(); err == nil {
		t.Error("expected error for missing TELEGRAM_TOKEN")
	}
	if err := (&Config{TelegramToken: "x"}).Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func equalInt64Slices(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

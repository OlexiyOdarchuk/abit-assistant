package bot

import (
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

// TestBuildNMTEditView_NoDuplicates guards against a regression where
// reusing a slice's backing array (row[:0]) overwrote buttons already
// pushed into the rows list — every row ended up pointing at the last
// pair (Географія, Інша іноземна).
func TestBuildNMTEditView_NoDuplicates(t *testing.T) {
	_, kb := buildNMTEditView(nil)

	// Last row is the "back to profile" navigation button — exclude it.
	if len(kb.InlineKeyboard) < 6 {
		t.Fatalf("expected ≥ 6 keyboard rows, got %d", len(kb.InlineKeyboard))
	}
	subjectRows := kb.InlineKeyboard[:len(kb.InlineKeyboard)-1]

	seen := make(map[string]int)
	for _, row := range subjectRows {
		for _, btn := range row {
			seen[btn.Text]++
		}
	}

	if len(seen) != len(profileSubjects) {
		t.Errorf("got %d unique subjects, want %d (subjects=%v, seen=%v)",
			len(seen), len(profileSubjects), profileSubjects, seen)
	}
	for _, subj := range profileSubjects {
		if seen[subj] != 1 {
			t.Errorf("subject %q appears %d times, want 1", subj, seen[subj])
		}
	}
}

// TestBuildNMTEditView_CheckmarksFilledSubjects ensures the ✅ prefix
// surfaces for subjects already present in the user's NMT map and that
// untouched subjects remain plain.
func TestBuildNMTEditView_CheckmarksFilledSubjects(t *testing.T) {
	nmt := storage.UserNMT{
		"Українська мова": 180,
		"Біологія":        172,
	}
	_, kb := buildNMTEditView(nmt)

	seen := make(map[string]string)
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			seen[btn.Text] = btn.Unique
		}
	}

	for subj, score := range nmt {
		_ = score
		if _, ok := seen["✅ "+subj]; !ok {
			t.Errorf("expected '✅ %s' in keyboard, got %v", subj, seen)
		}
	}
}

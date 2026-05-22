package bot

import (
	"strings"
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

	// Each subject must appear exactly once across all rows. Labels may
	// be prefixed with 🔒 (required) or ✅ (filled), so we check by
	// substring.
	for _, subj := range profileSubjects {
		count := 0
		for _, row := range subjectRows {
			for _, btn := range row {
				if strings.Contains(btn.Text, subj) {
					count++
				}
			}
		}
		if count != 1 {
			t.Errorf("subject %q appears %d times in keyboard, want 1", subj, count)
		}
	}

	// Total non-navigation buttons should equal the subject count.
	total := 0
	for _, row := range subjectRows {
		total += len(row)
	}
	if total != len(profileSubjects) {
		t.Errorf("got %d subject buttons, want %d", total, len(profileSubjects))
	}
}

// TestBuildNMTEditView_CheckmarksFilledSubjects ensures the ✅ marker
// surfaces for subjects already present in the user's NMT map.
func TestBuildNMTEditView_CheckmarksFilledSubjects(t *testing.T) {
	nmt := storage.UserNMT{
		"Українська мова": 180,
		"Біологія":        172,
	}
	_, kb := buildNMTEditView(nmt)

	wantFilled := map[string]bool{
		"Українська мова": true,
		"Біологія":        true,
	}
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			for subj, mustHaveCheck := range wantFilled {
				if !strings.Contains(btn.Text, subj) {
					continue
				}
				hasCheck := strings.Contains(btn.Text, "✅")
				if mustHaveCheck && !hasCheck {
					t.Errorf("filled subject %q has no ✅: %q", subj, btn.Text)
				}
				delete(wantFilled, subj)
			}
		}
	}
	if len(wantFilled) > 0 {
		t.Errorf("missing buttons for: %v", wantFilled)
	}
}

// TestBuildNMTEditView_RequiredLocked verifies that the three required
// НМТ subjects carry the 🔒 marker so the user sees them as mandatory.
func TestBuildNMTEditView_RequiredLocked(t *testing.T) {
	_, kb := buildNMTEditView(nil)

	wantLocked := map[string]bool{
		"Українська мова": true,
		"Математика":      true,
		"Історія України": true,
	}
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			for subj := range wantLocked {
				if !strings.Contains(btn.Text, subj) {
					continue
				}
				if !strings.Contains(btn.Text, "🔒") {
					t.Errorf("required %q missing 🔒: %q", subj, btn.Text)
				}
				delete(wantLocked, subj)
			}
		}
	}
	if len(wantLocked) > 0 {
		t.Errorf("required subjects not found: %v", wantLocked)
	}
}

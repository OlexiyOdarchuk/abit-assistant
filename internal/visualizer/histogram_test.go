package visualizer

import (
	"bytes"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

// pngHeader is the 8-byte signature every valid PNG file begins with.
var pngHeader = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func TestHistogram_RendersPNG(t *testing.T) {
	abits := []abit.Abiturient{
		{ID: 1, Score: 142},
		{ID: 2, Score: 145},
		{ID: 3, Score: 158},
		{ID: 4, Score: 161},
		{ID: 5, Score: 178},
		{ID: 6, Score: 190},
	}
	out, err := Histogram(abits, 165, 5)
	if err != nil {
		t.Fatalf("Histogram: %v", err)
	}
	if len(out) < 100 {
		t.Fatalf("PNG too small (%d bytes)", len(out))
	}
	if !bytes.HasPrefix(out, pngHeader) {
		t.Errorf("output is not a PNG (header=%x)", out[:8])
	}
}

func TestHistogram_NoUserScore(t *testing.T) {
	// userScore=0 → neutral color; still valid PNG.
	abits := []abit.Abiturient{
		{Score: 150}, {Score: 155}, {Score: 155}, {Score: 160},
		{Score: 165}, {Score: 170}, {Score: 175},
	}
	out, err := Histogram(abits, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, pngHeader) {
		t.Error("not a PNG")
	}
}

func TestHistogram_EmptyError(t *testing.T) {
	if _, err := Histogram(nil, 0, 5); err == nil {
		t.Error("expected error for empty list")
	}
}

func TestHistogram_DefaultBucketSize(t *testing.T) {
	abits := []abit.Abiturient{
		{Score: 150}, {Score: 155}, {Score: 155},
		{Score: 160}, {Score: 165}, {Score: 170},
	}
	out, err := Histogram(abits, 0, 0) // 0 → default 5
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, pngHeader) {
		t.Error("not a PNG")
	}
}

package desktop

import (
	"context"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/apidto"
)

// TestCore_Wiring constructs the full backend over an in-memory cache and
// exercises the offline use cases (filters, empty predict) — proving the
// service graph wires up and returns the expected shapes without launching a
// browser.
func TestCore_Wiring(t *testing.T) {
	cache, err := OpenCache(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	core := NewCore(cache, nil)

	f, err := core.GetFilters(context.Background())
	if err != nil {
		t.Fatalf("GetFilters: %v", err)
	}
	if len(f.Regions) == 0 || len(f.Industries) == 0 {
		t.Fatalf("expected populated filters, got %d regions / %d industries",
			len(f.Regions), len(f.Industries))
	}

	// Predict with no URLs is offline and must return the empty sentinel.
	pr, err := core.Predict(context.Background(), nil, apidto.Profile{}, false)
	if err != nil {
		t.Fatalf("Predict(empty): %v", err)
	}
	if pr.AdmittedIndex != -1 || len(pr.Items) != 0 {
		t.Fatalf("empty predict: got admitted=%d items=%d", pr.AdmittedIndex, len(pr.Items))
	}
}

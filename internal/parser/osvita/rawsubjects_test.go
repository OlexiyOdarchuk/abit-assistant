package osvita

import (
	"encoding/json"
	"testing"
)

// TestRawSubjectsUnmarshal locks in the live-data fix: osvita serves
// requests_subjects as `[]` (empty array) when a page has none, instead of
// `{}` — decoding that into a map used to fail and sink the whole page.
func TestRawSubjectsUnmarshal(t *testing.T) {
	cases := []struct {
		in      string
		wantLen int
	}{
		{`{}`, 0},
		{`[]`, 0},   // the live bug
		{`null`, 0}, // defensive
		{`{"123":{"100":[180,0,0]}}`, 1},
	}
	for _, c := range cases {
		var rc rawChunk
		body := `{"requests":[],"requests_subjects":` + c.in + `}`
		if err := json.Unmarshal([]byte(body), &rc); err != nil {
			t.Errorf("unmarshal %s: %v", c.in, err)
			continue
		}
		if len(rc.Subjects) != c.wantLen {
			t.Errorf("requests_subjects=%s → len %d, want %d", c.in, len(rc.Subjects), c.wantLen)
		}
	}
}

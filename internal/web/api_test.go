package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage/pgtest"
)

// --- fakes ---

type fakeSource struct{ prog *abit.Program }

func (f fakeSource) Parse(_ context.Context, _ string) (*abit.Program, error) { return f.prog, nil }
func (f fakeSource) ID() string                                               { return "fake" }

type fakeBrowser struct {
	progs   []osvita.SpecProgram
	filters osvita.Filters
}

func (f fakeBrowser) BrowsePrograms(_ context.Context, _ osvita.SpecFilter) ([]osvita.SpecProgram, error) {
	return f.progs, nil
}
func (f fakeBrowser) FetchFilters(_ context.Context) (osvita.Filters, error) { return f.filters, nil }

type fakeSearcher struct{}

func (fakeSearcher) Search(_ context.Context, _ string) ([]abit.ApplicantEntry, error) {
	return nil, abit.ErrNoData
}
func (fakeSearcher) ID() string { return "fake" }

func testProgram(budget int) *abit.Program {
	p := &abit.Program{
		EB: 40, OKR: 1, K4Max: 0.35, RK: 1.0,
		UniversityName: "Тестовий університет", ProgramName: "Комп'ютерні науки",
		Subjects: []abit.SubjectMeta{
			{ID: 1, Name: "Українська мова", Coefficient: 0.3},
			{ID: 2, Name: "Математика", Coefficient: 0.5},
			{ID: 3, Name: "Історія України", Coefficient: 0.2},
		},
		Volume: map[string]string{},
	}
	if budget > 0 {
		p.Volume["Максимальний обсяг державного замовлення"] = strconv.Itoa(budget)
	}
	return p
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := pgtest.New(t)

	prog := testProgram(50)
	programSvc := service.NewProgramService(fakeSource{prog: prog}, store, time.Hour)
	browser := fakeBrowser{
		progs: []osvita.SpecProgram{{URL: "https://x/y2025/r21/34/1/", University: "Тестовий університет", Specialty: "F3 Комп'ютерні науки", Budget: true}},
		filters: osvita.Filters{
			Regions:    []osvita.FilterOption{{Code: 21, Name: "Харківська область"}},
			Industries: []osvita.FilterOption{{Code: 166, Name: "Інформаційні технології"}},
		},
	}
	discoverSvc := service.NewDiscoverService(browser, programSvc, 4)
	applicantSvc := service.NewApplicantService(fakeSearcher{}, store, time.Hour)
	simSvc := service.NewPrioritySimulator(applicantSvc, nil, nil, 2, 10)

	return New(Deps{Program: programSvc, Discover: discoverSvc, Simulate: simSvc, Applicant: applicantSvc})
}

func do(t *testing.T, srv *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func TestAPI_Health(t *testing.T) {
	w := do(t, newTestServer(t), "GET", "/api/health", "")
	if w.Code != 200 {
		t.Fatalf("health code = %d", w.Code)
	}
}

func TestAPI_Filters(t *testing.T) {
	w := do(t, newTestServer(t), "GET", "/api/filters", "")
	if w.Code != 200 {
		t.Fatalf("filters code = %d", w.Code)
	}
	var resp filtersResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Served from static tables: 11 галузі (each with an A–K letter), 25 regions.
	if len(resp.Industries) != 11 {
		t.Errorf("industries = %d, want 11", len(resp.Industries))
	}
	for _, ind := range resp.Industries {
		if ind.Letter == "" {
			t.Errorf("industry %d has no letter", ind.Code)
		}
		if ind.Code == 166 && ind.Letter != "F" {
			t.Errorf("ІТ (166) letter = %q, want F", ind.Letter)
		}
	}
	if len(resp.Regions) != 25 {
		t.Errorf("regions = %d, want 25", len(resp.Regions))
	}
	if find(resp.Regions, 21) != "Харківська область" {
		t.Errorf("region 21 = %q, want Харківська область", find(resp.Regions, 21))
	}
}

func find(regions []optionDTO, code int) string {
	for _, r := range regions {
		if r.Code == code {
			return r.Name
		}
	}
	return ""
}

func TestAPI_Analyze(t *testing.T) {
	body := `{"url":"https://x/y2025/r21/34/1/","profile":{"nmt":{"Українська мова":180,"Математика":190,"Історія України":175}}}`
	w := do(t, newTestServer(t), "POST", "/api/analyze", body)
	if w.Code != 200 {
		t.Fatalf("analyze code = %d body=%s", w.Code, w.Body)
	}
	var resp analyzeResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.UserScore <= 0 {
		t.Errorf("userScore not computed: %v", resp.UserScore)
	}
	if resp.Program.University != "Тестовий університет" {
		t.Errorf("program meta wrong: %+v", resp.Program)
	}
	if resp.Analysis.BudgetTotal != 50 {
		t.Errorf("budget total = %d, want 50", resp.Analysis.BudgetTotal)
	}
}

func TestAPI_Discover(t *testing.T) {
	body := `{"galuz":166,"regions":[21],"budgetOnly":true,"profile":{"nmt":{"Українська мова":180,"Математика":190,"Історія України":175}}}`
	w := do(t, newTestServer(t), "POST", "/api/discover", body)
	if w.Code != 200 {
		t.Fatalf("discover code = %d body=%s", w.Code, w.Body)
	}
	var resp discoverResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Found != 1 || len(resp.Matches) != 1 {
		t.Fatalf("found=%d matches=%d, want 1/1", resp.Found, len(resp.Matches))
	}
	if resp.Matches[0].University != "Тестовий університет" || resp.Matches[0].Emoji == "" {
		t.Errorf("match wrong: %+v", resp.Matches[0])
	}
}

func TestAPI_BadRequest(t *testing.T) {
	w := do(t, newTestServer(t), "POST", "/api/analyze", `{"bogus":true}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad body code = %d, want 400", w.Code)
	}
}

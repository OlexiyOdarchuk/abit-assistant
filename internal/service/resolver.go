package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"unicode"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
)

// uniBrowser is the slice of *osvita.Parser the resolver needs: the static
// university directory plus /spec/ enumeration filtered by university.
type uniBrowser interface {
	FetchUniversities(ctx context.Context) ([]osvita.University, error)
	BrowsePrograms(ctx context.Context, f osvita.SpecFilter) ([]osvita.SpecProgram, error)
}

// Resolver maps a free-text (university, specialty) pair — as it appears on
// abit-poisk — to a concrete osvita program URL. It is the bridge the
// priority simulator needs to fetch a competitor's other programs.
//
// University name → universityId is resolved through the osvita directory
// (reliable). The university's programs are then listed once (/spec/ filtered
// by universityId, cached) and matched by specialty name. Matching across two
// sources' naming is imperfect, so a miss returns ok=false and the caller
// simply skips the prediction — it never guesses.
type Resolver struct {
	browser uniBrowser
	log     *slog.Logger

	mu    sync.Mutex
	dir   []osvita.University          // cached on first success only
	byUni map[int][]osvita.SpecProgram // universityId → its programs (cached)
}

// NewResolver wires the resolver over an osvita parser.
func NewResolver(browser uniBrowser) *Resolver {
	return &Resolver{
		browser: browser,
		log:     slog.Default().With("service", "resolver"),
		byUni:   map[int][]osvita.SpecProgram{},
	}
}

// WithLogger overrides the default logger.
func (r *Resolver) WithLogger(l *slog.Logger) *Resolver {
	r.log = l.With("service", "resolver")
	return r
}

// Resolve returns the osvita program URL for a university+specialty named the
// abit-poisk way. ok is false when the university can't be matched, its
// program list can't be loaded, or no program's specialty corresponds — in
// every such case the caller should skip rather than guess.
func (r *Resolver) Resolve(ctx context.Context, university, specialty string) (string, bool) {
	dir, err := r.directory(ctx)
	if err != nil {
		return "", false
	}
	uni, ok := osvita.MatchUniversity(dir, university)
	if !ok {
		return "", false
	}
	progs, err := r.programsOf(ctx, uni.ID)
	if err != nil || len(progs) == 0 {
		return "", false
	}
	want := normSpec(specialty)
	if want == "" {
		return "", false
	}
	// Prefer an exact specialty-name match; only fall back to a token-subset
	// match so a generic word ("право") can't latch onto a different
	// specialty ("правознавство") the way a raw substring would.
	for _, p := range progs {
		if normSpec(p.Specialty) == want {
			return p.URL, true
		}
	}
	for _, p := range progs {
		if specMatches(normSpec(p.Specialty), want) {
			return p.URL, true
		}
	}
	return "", false
}

func (r *Resolver) directory(ctx context.Context) ([]osvita.University, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dir != nil {
		return r.dir, nil
	}
	// Cache only on success — a transient fetch error must not permanently
	// disable the resolver.
	dir, err := r.browser.FetchUniversities(ctx)
	if err != nil {
		return nil, err
	}
	r.dir = dir
	return r.dir, nil
}

func (r *Resolver) programsOf(ctx context.Context, universityID int) ([]osvita.SpecProgram, error) {
	r.mu.Lock()
	if cached, ok := r.byUni[universityID]; ok {
		r.mu.Unlock()
		return cached, nil
	}
	r.mu.Unlock()

	progs, err := r.browser.BrowsePrograms(ctx, osvita.SpecFilter{University: universityID})
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.byUni[universityID] = progs
	r.mu.Unlock()
	return progs, nil
}

// normSpec normalizes a specialty label for comparison: lowercased, the
// leading letter-number code dropped (osvita prefixes "F3 …"; abit-poisk may
// not), punctuation removed, whitespace collapsed.
func normSpec(s string) string {
	s = strings.TrimSpace(s)
	// Drop a leading code token like "F3" / "I10" if followed by a space.
	if i := strings.IndexByte(s, ' '); i > 0 && i <= 4 && isCodeToken(s[:i]) {
		s = s[i+1:]
	}
	s = strings.ToLower(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func isCodeToken(s string) bool {
	if len(s) < 2 {
		return false
	}
	if !unicode.IsLetter(rune(s[0])) {
		return false
	}
	for _, c := range s[1:] {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

// specMatches reports whether two normalized specialty names correspond via
// token containment: every word of the shorter name must appear as a whole
// word in the longer one. This handles abbreviations / extra qualifiers
// ("комп ютерні науки" vs "прикладні комп ютерні науки") without the
// false positives a raw substring check produces (a word like "право" is NOT
// a token of "правознавство", so they won't match).
func specMatches(a, b string) bool {
	at, bt := strings.Fields(a), strings.Fields(b)
	if len(at) == 0 || len(bt) == 0 {
		return false
	}
	short, long := at, bt
	if len(short) > len(long) {
		short, long = long, short
	}
	set := make(map[string]struct{}, len(long))
	for _, t := range long {
		set[t] = struct{}{}
	}
	for _, t := range short {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}

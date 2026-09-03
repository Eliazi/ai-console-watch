package usage

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Tokens struct {
	In, Out, CacheWrite, CacheRead int64
}

func (t Tokens) Total() int64 {
	return t.In + t.Out + t.CacheWrite + t.CacheRead
}

type Bucket struct {
	Requests int
	Tokens   int64
	Cost     float64
}

type Day struct {
	Cost   float64
	Tokens int64
}

type Stats struct {
	Files       int
	Requests    int
	Sessions    map[string]struct{}
	Tok         Tokens
	Cost        float64
	ByModel     map[string]*Bucket
	ByProject   map[string]*Bucket
	ByDay       map[string]*Day
	MonthCost   float64
	MonthTokens int64
	First       time.Time
	Last        time.Time
}

func (s Stats) SessionCount() int {
	return len(s.Sessions)
}

func (s Stats) HasRange() bool {
	return !s.First.IsZero() && !s.Last.IsZero()
}

type event struct {
	Message   *message `json:"message"`
	Usage     *tokens  `json:"usage"`
	Model     string   `json:"model"`
	SessionID string   `json:"sessionId"`
	UUID      string   `json:"uuid"`
	RequestID string   `json:"requestId"`
	Timestamp string   `json:"timestamp"`
}

type message struct {
	ID    string  `json:"id"`
	Model string  `json:"model"`
	Usage *tokens `json:"usage"`
}

type tokens struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

func Collect(projectsDir string, now time.Time, monthOnly bool) (Stats, error) {
	s := Stats{
		Sessions:  map[string]struct{}{},
		ByModel:   map[string]*Bucket{},
		ByProject: map[string]*Bucket{},
		ByDay:     map[string]*Day{},
	}
	files, err := listJSONL(projectsDir)
	if err != nil {
		return s, err
	}
	s.Files = len(files)
	seen := map[string]struct{}{}
	for _, file := range files {
		project := decodeProject(projectsDir, file)
		if err := scanFile(file, project, now, monthOnly, seen, &s); err != nil {
			return s, err
		}
	}
	return s, nil
}

func listJSONL(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func decodeProject(projectsDir, filePath string) string {
	rel, err := filepath.Rel(projectsDir, filePath)
	if err != nil {
		return "unknown"
	}
	slug, _, _ := strings.Cut(rel, string(os.PathSeparator))
	if slug == "" {
		return "unknown"
	}
	parts := strings.FieldsFunc(strings.TrimPrefix(slug, "-"), func(r rune) bool { return r == '-' })
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return slug
}

func scanFile(path, project string, now time.Time, monthOnly bool, seen map[string]struct{}, s *Stats) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var ev event
		if json.Unmarshal([]byte(raw), &ev) != nil {
			continue
		}
		ingest(ev, project, now, monthOnly, seen, s)
	}
	return sc.Err()
}

func ingest(ev event, project string, now time.Time, monthOnly bool, seen map[string]struct{}, s *Stats) {
	u := ev.Usage
	model := ev.Model
	id := ev.RequestID
	if ev.UUID != "" && id == "" {
		id = ev.UUID
	}
	if ev.Message != nil {
		if ev.Message.Usage != nil {
			u = ev.Message.Usage
		}
		if ev.Message.Model != "" {
			model = ev.Message.Model
		}
		if ev.Message.ID != "" {
			id = ev.Message.ID
		}
	}
	if u == nil {
		return
	}
	if model == "" {
		model = "unknown"
	}
	tin, tout, tcw, tcr := u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens
	if tin == 0 && tout == 0 && tcw == 0 && tcr == 0 {
		return
	}

	ts, ok := parseTS(ev.Timestamp)
	if monthOnly && (!ok || !sameUTCMonth(ts, now)) {
		return
	}
	if id != "" {
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
	}

	cost := PriceFor(model).CostUSD(tin, tout, tcw, tcr)
	s.Requests++
	if ev.SessionID != "" {
		s.Sessions[ev.SessionID] = struct{}{}
	}
	s.Tok.In += tin
	s.Tok.Out += tout
	s.Tok.CacheWrite += tcw
	s.Tok.CacheRead += tcr
	s.Cost += cost
	addBucket(s.ByModel, model, tin+tout+tcw+tcr, cost)
	addBucket(s.ByProject, project, tin+tout+tcw+tcr, cost)

	if !ok {
		return
	}
	if s.First.IsZero() || ts.Before(s.First) {
		s.First = ts
	}
	if s.Last.IsZero() || ts.After(s.Last) {
		s.Last = ts
	}
	day := ts.UTC().Format("2006-01-02")
	d := s.ByDay[day]
	if d == nil {
		d = &Day{}
		s.ByDay[day] = d
	}
	d.Cost += cost
	d.Tokens += tin + tout + tcw + tcr
	if sameUTCMonth(ts, now) {
		s.MonthCost += cost
		s.MonthTokens += tin + tout + tcw + tcr
	}
}

func parseTS(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func sameUTCMonth(ts, now time.Time) bool {
	a, b := ts.UTC(), now.UTC()
	return a.Year() == b.Year() && a.Month() == b.Month()
}

func addBucket(m map[string]*Bucket, key string, tokens int64, cost float64) {
	b := m[key]
	if b == nil {
		b = &Bucket{}
		m[key] = b
	}
	b.Requests++
	b.Tokens += tokens
	b.Cost += cost
}

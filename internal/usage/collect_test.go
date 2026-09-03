package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPriceFor(t *testing.T) {
	if p := PriceFor("claude-opus-4-6"); p.In != 15 || p.Out != 75 {
		t.Fatalf("opus: %+v", p)
	}
	if p := PriceFor("claude-opus-5"); p.In != 15 {
		t.Fatalf("opus-5: %+v", p)
	}
	if p := PriceFor("claude-sonnet-5"); p.In != 3 {
		t.Fatalf("sonnet-5: %+v", p)
	}
	if p := PriceFor("mystery"); p.In != 3 {
		t.Fatalf("default: %+v", p)
	}
}

func TestCollectDedupAndMonth(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-home-eiazisraeli-repos-tasks-move-delivery-2-go")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	body := `{"type":"assistant","timestamp":"2026-09-01T10:00:00.000Z","sessionId":"s1","uuid":"u1","requestId":"r1","message":{"id":"msg_1","model":"claude-sonnet-5","usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"assistant","timestamp":"2026-09-01T10:00:01.000Z","sessionId":"s1","uuid":"u2","requestId":"r1","message":{"id":"msg_1","model":"claude-sonnet-5","usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"assistant","timestamp":"2026-08-01T10:00:00.000Z","sessionId":"s2","uuid":"u3","requestId":"r2","message":{"id":"msg_2","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"user","timestamp":"2026-09-01T10:00:02.000Z","sessionId":"s1"}
`
	if err := os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Collect(root, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Files != 1 {
		t.Fatalf("files=%d", s.Files)
	}
	if s.Requests != 2 {
		t.Fatalf("requests=%d (want 2 after dedup)", s.Requests)
	}
	if s.SessionCount() != 2 {
		t.Fatalf("sessions=%d", s.SessionCount())
	}
	if _, ok := s.ByProject["2/go"]; !ok {
		t.Fatalf("project keys: %v", keys(s.ByProject))
	}
	// sonnet: (1000*3 + 500*15)/1e6 = 0.0105
	// opus: (100*15 + 100*75)/1e6 = 0.009
	if abs(s.MonthCost-0.0105) > 1e-9 {
		t.Fatalf("monthCost=%v", s.MonthCost)
	}
	if abs(s.Cost-0.0195) > 1e-9 {
		t.Fatalf("cost=%v", s.Cost)
	}
	if s.ByModel["claude-sonnet-5"].Requests != 1 || s.ByModel["claude-opus-4-6"].Requests != 1 {
		t.Fatalf("byModel=%v %v", s.ByModel["claude-sonnet-5"], s.ByModel["claude-opus-4-6"])
	}

	month, err := Collect(root, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if month.Requests != 1 {
		t.Fatalf("month requests=%d", month.Requests)
	}
	if month.SessionCount() != 1 {
		t.Fatalf("month sessions=%d", month.SessionCount())
	}
	if _, ok := month.ByModel["claude-opus-4-6"]; ok {
		t.Fatal("opus from prior month should be excluded")
	}
	if abs(month.Cost-0.0105) > 1e-9 {
		t.Fatalf("month cost=%v", month.Cost)
	}
}

func keys(m map[string]*Bucket) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Eliazi/ai-console-watch/internal/usage"
)

func TestRenderIncludesSections(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	s := usage.Stats{
		Files:    3,
		Requests: 10,
		Sessions: map[string]struct{}{"a": {}, "b": {}},
		Tok:      usage.Tokens{In: 1500, Out: 200, CacheWrite: 100, CacheRead: 50},
		Cost:     1.25,
		ByModel: map[string]*usage.Bucket{
			"claude-sonnet-5": {Requests: 8, Tokens: 1000, Cost: 1.0},
		},
		ByProject: map[string]*usage.Bucket{
			"ai-console/watch": {Requests: 8, Tokens: 1000, Cost: 1.0},
		},
		ByDay:       map[string]*usage.Day{now.Format("2006-01-02"): {Cost: 0.4, Tokens: 100}},
		MonthCost:   0.4,
		MonthTokens: 100,
		First:       now.Add(-48 * time.Hour),
		Last:        now,
	}
	out := Render(s, Options{Budget: 50, Watch: true, Refresh: 5 * time.Second, Now: now})
	for _, want := range []string{
		"CLAUDE CODE — USAGE DASHBOARD",
		"OVERVIEW",
		"BY MODEL",
		"THIS MONTH",
		"TOP PROJECTS",
		"sonnet-5",
		"ai-console/watch",
		"$50",
		"Ctrl+C",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}

	monthOut := Render(s, Options{Budget: 50, MonthOnly: true, Now: now})
	if !strings.Contains(monthOut, "September 2026 only") {
		t.Fatalf("missing month label:\n%s", monthOut)
	}
	if strings.Contains(monthOut, "THIS MONTH") {
		t.Fatalf("THIS MONTH should be omitted when -month:\n%s", monthOut)
	}
}

func TestFmtNum(t *testing.T) {
	if fmtNum(500) != "500" {
		t.Fatalf("%s", fmtNum(500))
	}
	if fmtNum(1500) != "1.5K" {
		t.Fatalf("%s", fmtNum(1500))
	}
}

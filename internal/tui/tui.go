package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Eliazi/ai-console-watch/internal/usage"
)

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[90m"
	cyan   = "\x1b[36m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	mag    = "\x1b[35m"
	blue   = "\x1b[34m"
	white  = "\x1b[97m"
	width  = 66
)

var sparkChars = []rune("▁▂▃▄▅▆▇█")

type Options struct {
	Budget    float64
	Watch     bool
	Refresh   time.Duration
	Now       time.Time
	MonthOnly bool
}

func Render(s usage.Stats, opt Options) string {
	var b strings.Builder
	now := opt.Now
	if now.IsZero() {
		now = time.Now()
	}

	b.WriteString(line("╭", "─", "╮") + "\n")
	b.WriteString(center(bold+white+"CLAUDE CODE — USAGE DASHBOARD"+reset) + "\n")
	if opt.MonthOnly {
		b.WriteString(center(dim+now.UTC().Format("January 2006")+" only · "+now.Format("2006-01-02 15:04:05")+reset) + "\n")
	} else {
		b.WriteString(center(dim+now.Format("2006-01-02 15:04:05")+reset) + "\n")
	}
	b.WriteString(line("├", "─", "┤") + "\n")

	b.WriteString(row("  "+bold+"OVERVIEW"+reset) + "\n")
	b.WriteString(row(fmt.Sprintf("  %sRequests%s  %-10d %sSessions%s %-8d %sLogs%s %d",
		dim, reset, s.Requests, dim, reset, s.SessionCount(), dim, reset, s.Files)) + "\n")
	b.WriteString(row(fmt.Sprintf("  %sTokens%s    %s%-10s%s %sEst. cost%s %s%s%s",
		dim, reset, white, fmtNum(s.Tok.Total()), reset, dim, reset, green, fmtUSD(s.Cost), reset)) + "\n")
	b.WriteString(row(fmt.Sprintf("  %sin %s  out %s  cache-w %s  cache-r %s%s",
		dim, fmtNum(s.Tok.In), fmtNum(s.Tok.Out), fmtNum(s.Tok.CacheWrite), fmtNum(s.Tok.CacheRead), reset)) + "\n")
	if s.HasRange() {
		b.WriteString(row(fmt.Sprintf("  %sRange     %s → %s%s",
			dim, s.First.UTC().Format("2006-01-02"), s.Last.UTC().Format("2006-01-02"), reset)) + "\n")
	}
	b.WriteString(line("├", "─", "┤") + "\n")

	b.WriteString(row("  "+bold+"BY MODEL"+reset) + "\n")
	models := sortBuckets(s.ByModel)
	if len(models) == 0 {
		b.WriteString(row("  "+dim+"no data"+reset) + "\n")
	}
	if len(models) > 6 {
		models = models[:6]
	}
	for _, item := range models {
		pct := 0.0
		if s.Cost > 0 {
			pct = item.b.Cost / s.Cost * 100
		}
		short := strings.TrimPrefix(item.name, "claude-")
		if len(short) > 26 {
			short = short[:26]
		}
		b.WriteString(row(fmt.Sprintf("  %s%-27s%s%s %s", mag, short, reset, bar(pct, 16), padLeft(fmtUSD(item.b.Cost), 9))) + "\n")
		b.WriteString(row(fmt.Sprintf("  %s  %5d req   %s tokens%s", dim, item.b.Requests, fmtNum(item.b.Tokens), reset)) + "\n")
	}
	b.WriteString(line("├", "─", "┤") + "\n")

	if !opt.MonthOnly {
		b.WriteString(row("  "+bold+"THIS MONTH"+reset) + "\n")
		b.WriteString(row(fmt.Sprintf("  %sSpend%s %s%s%s   %sTokens%s %s",
			dim, reset, green, fmtUSD(s.MonthCost), reset, dim, reset, fmtNum(s.MonthTokens))) + "\n")
	}
	if opt.Budget > 0 {
		pct := s.MonthCost / opt.Budget * 100
		b.WriteString(row(fmt.Sprintf("  %s %3.0f%%  of %s", bar(pct, 30), pct, fmtUSD(opt.Budget))) + "\n")
	}
	days := make([]float64, 7)
	for i := 6; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).UTC().Format("2006-01-02")
		if v := s.ByDay[d]; v != nil {
			days[6-i] = v.Cost
		}
	}
	maxDay := 0.0
	for _, v := range days {
		if v > maxDay {
			maxDay = v
		}
	}
	b.WriteString(row(fmt.Sprintf("  %sLast 7 days%s  %s%s%s  %smax %s/day%s",
		dim, reset, cyan, sparkline(days), reset, dim, fmtUSD(maxDay), reset)) + "\n")
	b.WriteString(line("├", "─", "┤") + "\n")

	b.WriteString(row("  "+bold+"TOP PROJECTS"+reset) + "\n")
	projects := sortBuckets(s.ByProject)
	if len(projects) == 0 {
		b.WriteString(row("  "+dim+"no data"+reset) + "\n")
	}
	if len(projects) > 5 {
		projects = projects[:5]
	}
	for _, item := range projects {
		pct := 0.0
		if s.Cost > 0 {
			pct = item.b.Cost / s.Cost * 100
		}
		name := item.name
		if len(name) > 27 {
			name = name[:27]
		}
		b.WriteString(row(fmt.Sprintf("  %s%-27s%s%s %s", blue, name, reset, bar(pct, 16), padLeft(fmtUSD(item.b.Cost), 9))) + "\n")
	}

	b.WriteString(line("╰", "─", "╯") + "\n")
	b.WriteString(dim + "  Estimates use Anthropic list prices. On Pro/Max plans treat as API-equivalent value." + reset + "\n")
	if opt.Watch {
		sec := opt.Refresh / time.Second
		if sec == 0 {
			sec = 5
		}
		b.WriteString(fmt.Sprintf("%s  Ctrl+C to exit — refreshing every %ds%s\n", dim, sec, reset))
	}
	return b.String()
}

type namedBucket struct {
	name string
	b    *usage.Bucket
}

func sortBuckets(m map[string]*usage.Bucket) []namedBucket {
	out := make([]namedBucket, 0, len(m))
	for k, v := range m {
		out = append(out, namedBucket{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].b.Cost > out[j].b.Cost })
	return out
}

func strip(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			if j := strings.IndexByte(s[i:], 'm'); j >= 0 {
				i += j + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

func visibleLen(s string) int {
	return utf8.RuneCountInString(strip(s))
}

func line(l, m, r string) string {
	return cyan + l + strings.Repeat(m, width-2) + r + reset
}

func row(content string) string {
	pad := width - 2 - visibleLen(content)
	if pad < 0 {
		pad = 0
	}
	return cyan + "│" + reset + content + strings.Repeat(" ", pad) + cyan + "│" + reset
}

func center(text string) string {
	left := (width - 2 - visibleLen(text)) / 2
	if left < 0 {
		left = 0
	}
	return row(strings.Repeat(" ", left) + text)
}

func bar(pct float64, w int) string {
	p := math.Max(0, math.Min(100, pct))
	filled := int(math.Round(p * float64(w) / 100))
	color := green
	if p >= 85 {
		color = red
	} else if p >= 60 {
		color = yellow
	}
	return color + strings.Repeat("█", filled) + dim + strings.Repeat("░", w-filled) + reset
}

func sparkline(values []float64) string {
	max := 1.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range values {
		idx := int(math.Round(v / max * 7))
		if idx > 7 {
			idx = 7
		}
		if idx < 0 {
			idx = 0
		}
		b.WriteRune(sparkChars[idx])
	}
	return b.String()
}

func fmtNum(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.2fB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.2fM", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func fmtUSD(n float64) string {
	if n < 10 {
		return fmt.Sprintf("$%.3f", n)
	}
	return fmt.Sprintf("$%.2f", n)
}

func padLeft(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return strings.Repeat(" ", w-len(s)) + s
}

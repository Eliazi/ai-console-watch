package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/Eliazi/ai-console-watch/internal/tui"
	"github.com/Eliazi/ai-console-watch/internal/usage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	watch := flag.Bool("watch", false, "refresh every 5s")
	month := flag.Bool("month", false, "show metrics from the start of this month only")
	budgetFlag := flag.Float64("budget", -1, "monthly budget in USD")
	claudeDir := flag.String("claude-dir", "", "path to ~/.claude (or CLAUDE_DIR)")
	flag.Parse()

	dir := *claudeDir
	if dir == "" {
		dir = os.Getenv("CLAUDE_DIR")
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".claude")
	}
	projects := filepath.Join(dir, "projects")
	if st, err := os.Stat(projects); err != nil || !st.IsDir() {
		return fmt.Errorf("no Claude logs found at: %s\nSet CLAUDE_DIR=/path/to/.claude if yours lives elsewhere.", projects)
	}

	budget := *budgetFlag
	if budget < 0 {
		budget = 0
		if v := os.Getenv("BUDGET"); v != "" {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("BUDGET: %w", err)
			}
			budget = n
		}
	}

	refresh := 5 * time.Second
	opt := tui.Options{Budget: budget, Watch: *watch, Refresh: refresh, MonthOnly: *month}

	printFrame := func() error {
		stats, err := usage.Collect(projects, time.Now(), *month)
		if err != nil {
			return err
		}
		if *watch {
			fmt.Print("\x1b[2J\x1b[H")
		}
		fmt.Print(tui.Render(stats, opt))
		return nil
	}

	if err := printFrame(); err != nil {
		return err
	}
	if !*watch {
		return nil
	}

	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(refresh)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if err := printFrame(); err != nil {
				return err
			}
		case <-ch:
			fmt.Print("\n\x1b[90mbye\x1b[0m\n")
			return nil
		}
	}
}

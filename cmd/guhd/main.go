package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/dashboard"
	"github.com/cosgroveb/guhd/internal/theme"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	flags := pflag.NewFlagSet("guhd", pflag.ContinueOnError)
	flags.SetInterspersed(false)
	help := flags.BoolP("help", "h", false, "Show help")
	showVersion := flags.Bool("version", false, "Show version")
	path := flags.String("config", "", "Use another configuration file")
	themeName := flags.String("theme", "", "Use a named theme")
	preview := flags.Bool("preview", false, "Preview fictional dashboard data offline")
	setup := flags.Bool("setup", false, "Configure account, calendars, and projects")
	if err := parseFlags(flags, args); err != nil {
		_, _ = fmt.Fprintln(stderr, "guhd:", err)
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "guhd: unexpected positional arguments")
		return 2
	}
	if *help {
		if _, err := io.WriteString(stdout, `Usage: guhd [options]

Google unified heads up display.

  --setup         Configure account, calendars, and projects
  --config PATH   Use another configuration file
  --theme NAME    Use a named theme
  --preview       Preview fictional dashboard data offline
  -h, --help      Show help
  --version       Show version

Examples:
  guhd
  guhd --setup
  guhd --config work.json
  guhd --preview --theme moni-chrome
`); err != nil {
			_, _ = fmt.Fprintln(stderr, "guhd: write help:", err)
			return 1
		}
		return 0
	}
	if *showVersion {
		if _, err := fmt.Fprintf(stdout, "guhd %s\n", version); err != nil {
			_, _ = fmt.Fprintln(stderr, "guhd: write version:", err)
			return 1
		}
		return 0
	}
	if flags.Changed("theme") && !theme.ValidName(*themeName) {
		_, _ = fmt.Fprintln(stderr, "guhd: --theme requires a name containing letters, digits, hyphens or underscores")
		return 2
	}
	if *preview && (flags.Changed("setup") || flags.Changed("config")) {
		_, _ = fmt.Fprintln(stderr, "guhd: --preview cannot be combined with --setup or --config")
		return 2
	}
	explicit := flags.Changed("config")
	if explicit && *path == "" {
		_, _ = fmt.Fprintln(stderr, "guhd: --config requires a nonempty path")
		return 2
	}
	if !explicit && !*preview {
		var err error
		*path, err = config.DefaultPath()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "guhd:", err)
			return 1
		}
	}
	cfg := config.Defaults()
	var needsSetup bool
	if !*preview {
		var err error
		cfg, needsSetup, err = startupConfig(*path, explicit, *setup)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "guhd:", err)
			return 1
		}
	}
	selectedTheme := cfg.Theme
	if flags.Changed("theme") {
		selectedTheme = *themeName
	}
	styles, err := theme.Load(selectedTheme)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "guhd:", err)
		return 1
	}
	output, ok := stdout.(*os.File)
	if !ok || !term.IsTerminal(int(stdin.Fd())) || !term.IsTerminal(int(output.Fd())) {
		_, _ = fmt.Fprintln(stderr, "guhd: an interactive terminal is required; run guhd in a terminal (use --setup to configure)")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var model tea.Model
	if *preview {
		model = dashboard.NewPreview(ctx, styles)
	} else {
		model = dashboard.New(ctx, cfg, *path, needsSetup, styles)
	}
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(stdin), tea.WithOutput(stdout))
	if _, err := program.Run(); err != nil && ctx.Err() == nil {
		_, _ = fmt.Fprintln(stderr, "guhd:", err)
		return 1
	}
	return 0
}

func parseFlags(flags *pflag.FlagSet, args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		if strings.HasPrefix(arg, "--") {
			name, _, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			if flag := flags.Lookup(name); flag != nil && flag.NoOptDefVal == "" && !hasValue {
				i++
			}
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(arg, "-"), "=")
		var err error
		if flags.Lookup(name) != nil {
			err = fmt.Errorf("use --%s instead of -%s", name, name)
		} else if strings.HasPrefix(strings.TrimLeft(name, "h"), "test.") {
			// pflag skips test.* even after consuming -h in a shorthand group.
			err = fmt.Errorf("unknown flag: %s", arg)
		}
		if err != nil {
			if earlier := flags.Parse(args[:i]); earlier != nil {
				return earlier
			}
			return err
		}
	}
	return flags.Parse(args)
}

func startupConfig(path string, explicit, setup bool) (config.Config, bool, error) {
	cfg, err := config.Load(path)
	if errors.Is(err, os.ErrNotExist) && (!explicit || setup) {
		return config.Defaults(), true, nil
	}
	if err != nil {
		return config.Config{}, false, err
	}
	return cfg, setup, nil
}

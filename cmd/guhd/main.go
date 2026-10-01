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
  -h, --help      Show help
  --version       Show version

Examples:
  guhd
  guhd --setup
  guhd --config work.json
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
	explicit := flags.Changed("config")
	if explicit && *path == "" {
		_, _ = fmt.Fprintln(stderr, "guhd: --config requires a nonempty path")
		return 2
	}
	if !explicit {
		var err error
		*path, err = config.DefaultPath()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "guhd:", err)
			return 1
		}
	}
	cfg, needsSetup, err := startupConfig(*path, explicit, *setup)
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
	program := tea.NewProgram(dashboard.New(ctx, cfg, *path, needsSetup), tea.WithContext(ctx), tea.WithInput(stdin), tea.WithOutput(stdout))
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
		if arg == "--config" {
			i++
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

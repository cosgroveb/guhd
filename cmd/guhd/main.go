package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/dashboard"
	"golang.org/x/term"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("guhd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	help := flags.Bool("help", false, "print help and exit")
	flags.BoolVar(help, "h", false, "print help and exit")
	showVersion := flags.Bool("version", false, "print version and exit")
	path := flags.String("config", "", "configuration file path")
	setup := flags.Bool("setup", false, "configure accounts and dashboard settings")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: guhd [--config PATH] [--setup]\n\nGoogle unified heads up display.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "guhd: unexpected positional arguments")
		return 2
	}
	if *help {
		var usage bytes.Buffer
		flags.SetOutput(&usage)
		flags.Usage()
		if _, err := usage.WriteTo(stdout); err != nil {
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
	explicit := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicit = true
		}
	})
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

package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Gog struct {
	Executable string
	Timeout    time.Duration
}

const outputLimit = 8 << 20

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > b.limit-b.buffer.Len() {
		p = p[:b.limit-b.buffer.Len()]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func run(ctx context.Context, timeout time.Duration, executable string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.WaitDelay = time.Second
	stdout := boundedBuffer{limit: outputLimit}
	stderr := boundedBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s: %w", executable, ctx.Err())
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%s executable not found; install it and add it to PATH: %w", executable, err)
		}
		return nil, fmt.Errorf("%s: %w: %s", executable, err, strings.TrimSpace(stderr.buffer.String()))
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, fmt.Errorf("%s output exceeds size limit", executable)
	}
	return stdout.buffer.Bytes(), nil
}

func (g Gog) read(ctx context.Context, account Account, envelope string, target any, args ...string) error {
	executable := g.Executable
	if executable == "" {
		executable = "gog"
	}
	argv := []string{"--json", "--no-input", "--readonly", "--color", "never", "--wrap-untrusted=false"}
	if account.Email != "" {
		argv = append(argv, "--account", account.Email, "--client", account.Client)
	}
	data, err := run(ctx, g.Timeout, executable, append(argv, args...)...)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("gog %s: malformed JSON: %w", strings.Join(args, " "), err)
	}
	if _, ok := fields[envelope]; !ok {
		return fmt.Errorf("gog %s: missing %q envelope", strings.Join(args, " "), envelope)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("gog %s: invalid response: %w", strings.Join(args, " "), err)
	}
	return nil
}

func (g Gog) Accounts(ctx context.Context) ([]Account, error) {
	var response struct{ Accounts []Account }
	if err := g.read(ctx, Account{}, "accounts", &response, "auth", "list"); err != nil {
		return nil, err
	}
	if len(response.Accounts) == 0 {
		return nil, fmt.Errorf("no gog accounts; run gog auth add EMAIL --services gmail,calendar")
	}
	for i := range response.Accounts {
		if response.Accounts[i].Client == "" {
			response.Accounts[i].Client = "default"
		}
	}
	return response.Accounts, nil
}

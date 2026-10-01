package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This test binary also acts as gog, exercising the real argv and pipes.
func TestMain(m *testing.M) {
	if os.Getenv("GUHD_SOURCE_HELPER") == "1" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil || len(input) != 0 {
			fmt.Fprint(os.Stderr, "unexpected stdin")
			os.Exit(3)
		}
		if path := os.Getenv("GUHD_HELPER_ARGV"); path != "" {
			data, _ := json.Marshal(os.Args[1:])
			if err := os.WriteFile(path, data, 0600); err != nil {
				panic(err)
			}
		}
		if path := os.Getenv("GUHD_HELPER_ARGV_LOG"); path != "" {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				panic(err)
			}
			if err := json.NewEncoder(file).Encode(os.Args[1:]); err != nil {
				panic(err)
			}
			if err := file.Close(); err != nil {
				panic(err)
			}
		}
		if os.Getenv("GUHD_HELPER_ATTACHMENT") == "1" {
			for i, arg := range os.Args {
				if arg != "--out" || i+1 >= len(os.Args) {
					continue
				}
				path := os.Args[i+1]
				info, err := os.Stat(filepath.Dir(path))
				if err != nil || info.Mode().Perm() != 0700 {
					fmt.Fprint(os.Stderr, "attachment directory is not private")
					os.Exit(3)
				}
				if err := os.WriteFile(path, []byte("fictional downloaded bytes"), 0600); err != nil {
					panic(err)
				}
			}
		}
		switch os.Getenv("GUHD_HELPER_MODE") {
		case "fail":
			fmt.Fprint(os.Stderr, "fictional auth failure")
			os.Exit(7)
		case "wait":
			for {
				time.Sleep(time.Hour)
			}
		case "large":
			fmt.Print(strings.Repeat("x", outputLimit+1))
			os.Exit(0)
		}
		data := os.Getenv("GUHD_HELPER_JSON")
		if path := os.Getenv("GUHD_HELPER_JSON_FILE"); path != "" {
			content, err := os.ReadFile(path)
			if err != nil {
				panic(err)
			}
			data = string(content)
		}
		if len(os.Args) > 2 && strings.Contains(strings.Join(os.Args, " "), "calendar calendars") && os.Getenv("GUHD_HELPER_CALENDARS") != "" {
			data = os.Getenv("GUHD_HELPER_CALENDARS")
		}
		fmt.Print(data)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeGog(t *testing.T, response string) Gog {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GUHD_SOURCE_HELPER", "1")
	t.Setenv("GUHD_HELPER_JSON", response)
	return Gog{Executable: executable, Timeout: 5 * time.Second}
}

func TestAccountsBoundary(t *testing.T) {
	g := fakeGog(t, `{"accounts":[{"email":"alex@example.com","client":"work"},{"email":"bad@example.com","error":"locked"}]}`)
	accounts, err := g.Accounts(t.Context())
	if err != nil || len(accounts) != 2 || accounts[0].Client != "work" || accounts[1].Error != "locked" || accounts[1].Client != "default" {
		t.Fatalf("accounts=%+v err=%v", accounts, err)
	}
	for _, response := range []string{`{}`, `[]`, `{"accounts":false}`, `not-json`, `{"accounts":[]} `} {
		t.Setenv("GUHD_HELPER_JSON", response)
		if _, err := g.Accounts(t.Context()); err == nil {
			t.Errorf("response %q accepted", response)
		}
	}
}

func TestCommandFailures(t *testing.T) {
	g := fakeGog(t, `{"accounts":[]}`)
	t.Setenv("GUHD_HELPER_MODE", "fail")
	if _, err := g.Accounts(t.Context()); err == nil || !strings.Contains(err.Error(), "exit status 7: fictional auth failure") {
		t.Fatalf("error=%v", err)
	}
	t.Setenv("GUHD_HELPER_MODE", "large")
	if _, err := g.Accounts(t.Context()); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("error=%v", err)
	}
	t.Setenv("GUHD_HELPER_MODE", "wait")
	g.Timeout = 20 * time.Millisecond
	if _, err := g.Accounts(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := g.Accounts(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	g.Executable = "guhd-nonexistent-fictional-executable"
	if _, err := g.Accounts(t.Context()); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error=%v", err)
	}
}

func TestMessagesPaginationAndArgv(t *testing.T) {
	g := fakeGog(t, `{"messages":[{"id":"m1","threadId":"a/b?#","subject":"Review","labels":["UNREAD"],"date":"legacy"},{"id":"m2","internalDateIso":"2026-09-30T12:15:00Z"}],"nextPageToken":"next"}`)
	path := filepath.Join(t.TempDir(), "args.json")
	t.Setenv("GUHD_HELPER_ARGV", path)
	account := Account{Email: "alex+work@example.com", Client: "work"}
	page, err := g.Messages(t.Context(), account, "-in:trash subject:hello world", "opaque next")
	if err != nil || len(page.Messages) != 2 || page.NextPageToken != "next" || !page.Messages[0].Unread || !page.Messages[0].Received.IsZero() || page.Messages[1].Received.IsZero() {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if page.Messages[0].URL != "https://mail.google.com/mail/?authuser=alex%2Bwork%40example.com#all/a%2Fb%3F%23" {
		t.Fatal(page.Messages[0].URL)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(data, &args); err != nil {
		t.Fatal(err)
	}
	want := []string{"--json", "--no-input", "--readonly", "--color", "never", "--wrap-untrusted=false", "--account", account.Email, "--client", "work", "gmail", "messages", "search", "--max", "50", "--timezone", "UTC", "--page", "opaque next", "--", "-in:trash subject:hello world"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q", args)
	}
	for _, response := range []string{`{"messages":[]}`, `{"messages":null}`} {
		t.Setenv("GUHD_HELPER_JSON", response)
		page, err := g.Messages(t.Context(), account, "in:inbox", "")
		if err != nil || len(page.Messages) != 0 {
			t.Fatalf("page=%+v err=%v", page, err)
		}
	}
}

func TestCountsAndDetail(t *testing.T) {
	g := fakeGog(t, `{"label":{"messagesTotal":132,"messagesUnread":7,"threadsUnread":2}}`)
	account := Account{Email: "alex@example.com", Client: "default"}
	counts, err := g.Counts(t.Context(), account)
	if err != nil || counts != (MailCounts{Total: 132, Unread: 7}) {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
	t.Setenv("GUHD_HELPER_JSON", `{"label":{}}`)
	counts, err = g.Counts(t.Context(), account)
	if err != nil || counts != (MailCounts{}) {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
	for _, data := range []string{`{}`, `{"label":null}`} {
		t.Setenv("GUHD_HELPER_JSON", data)
		if _, err := g.Counts(t.Context(), account); err == nil {
			t.Fatal("invalid counts accepted")
		}
	}
	t.Setenv("GUHD_HELPER_JSON", `{"message":{"id":"m1","threadId":"t1","labelIds":["UNREAD"],"internalDate":"1790769600000","payload":{"mimeType":"text/html"}},"headers":{"from":"Casey","to":"Alex","subject":"Hello"},"body":"<p>Hello &amp; welcome</p><script>bad()</script><p>Next</p>"}`)
	detail, err := g.Detail(t.Context(), account, "m1")
	if err != nil || !detail.Message.Unread || detail.Message.Received.IsZero() || detail.Body != "Hello & welcome\n\nNext" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	t.Setenv("GUHD_HELPER_JSON", `{"message":{"id":"m1","snippet":"fallback"}}`)
	detail, err = g.Detail(t.Context(), account, "m1")
	if err != nil || detail.Body != "fallback" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

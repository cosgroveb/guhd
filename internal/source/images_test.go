package source

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDetailAttachments(t *testing.T) {
	g := fakeGog(t, `{"message":{"id":"m1","snippet":"still readable"},"attachments":[
		{"attachmentId":"document","filename":"file.pdf","mimeType":"application/pdf","size":8},
		{"attachmentId":"png","filename":"../../untrusted\u001b.png","mimeType":"IMAGE/PNG; name=photo","size":12},
		{"attachmentId":"jpeg","filename":"photo.jpg","mimeType":"image/jpeg","size":3145728},
		{"attachmentId":"gif","mimeType":"image/gif","size":12},
		{"attachmentId":"big","mimeType":"image/png","size":3145729},
		{"attachmentId":"webp","mimeType":"image/webp","size":12},
		{"attachmentId":"unknown","mimeType":"image/png","size":0},
		{"attachmentId":"negative","mimeType":"image/png","size":-1},
		{"mimeType":"image/png","size":12}]}`)
	detail, err := g.Detail(t.Context(), Account{}, "m1")
	if err != nil || detail.Body != "still readable" || len(detail.Attachments) != 8 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	for i, a := range detail.Attachments {
		if (a.Unavailable != "") != (i >= 3) {
			t.Errorf("attachment %d = %+v", i, a)
		}
	}
	if detail.Attachments[0].MIMEType != "image/png" || detail.Attachments[0].Name != "../../untrusted\x1b.png" {
		t.Fatalf("metadata = %+v", detail.Attachments[0])
	}
	var items []map[string]any
	for range 25 {
		items = append(items, map[string]any{"attachmentId": "a", "mimeType": "image/png", "size": 1})
	}
	data, err := json.Marshal(map[string]any{"message": map[string]string{"id": "m1"}, "attachments": items})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GUHD_HELPER_JSON", string(data))
	detail, err = g.Detail(t.Context(), Account{}, "m1")
	if err != nil || len(detail.Attachments) != 20 {
		t.Fatalf("attachment count=%d err=%v", len(detail.Attachments), err)
	}
}

func TestAttachmentArgumentsAndCleanup(t *testing.T) {
	g := fakeGog(t, `{"contentBase64":"aGVsbG8=","filename":"../../sender","path":"/untrusted"}`)
	argsPath := filepath.Join(t.TempDir(), "args.json")
	downloadDir := t.TempDir()
	t.Setenv("TMPDIR", downloadDir)
	t.Setenv("GUHD_HELPER_ARGV", argsPath)
	t.Setenv("GUHD_HELPER_ATTACHMENT", "1")
	account := Account{Email: "alex@example.com", Client: "work"}
	a := Attachment{ID: "-opaque attachment", Name: "../../sender\x1b", MIMEType: "image/png", Size: 5}
	data, err := g.Attachment(t.Context(), account, "-opaque message", a)
	if err != nil || string(data) != "hello" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	out := ""
	for i, arg := range args {
		if arg == "--out" && i+1 < len(args) {
			out = args[i+1]
		}
	}
	if filepath.Dir(filepath.Dir(out)) != downloadDir || filepath.Base(out) != "attachment" {
		t.Fatalf("output path=%q", out)
	}
	want := []string{"--json", "--no-input", "--readonly", "--color", "never", "--wrap-untrusted=false", "--account", account.Email, "--client", account.Client, "gmail", "attachment", "--use-indexed-attachment-ids=false", "--inline", "--inline-max-bytes", "3145728", "--out", out, "--", "-opaque message", "-opaque attachment"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q want=%q", args, want)
	}
	assertEmptyDirectory(t, downloadDir)
}

func TestAttachmentRejectsUnavailableBeforeCommand(t *testing.T) {
	g := Gog{Executable: "must-not-run"}
	for _, a := range []Attachment{
		{ID: "a", MIMEType: "image/png", Size: 0},
		{ID: "a", MIMEType: "image/png", Size: -1},
		{ID: "a", MIMEType: "image/png", Size: attachmentLimit + 1},
		{ID: "a", MIMEType: "image/svg+xml", Size: 5},
		{MIMEType: "image/png", Size: 5},
	} {
		if _, err := g.Attachment(t.Context(), Account{}, "m", a); err == nil || !strings.Contains(err.Error(), "attachment unavailable") {
			t.Errorf("attachment=%+v error=%v", a, err)
		}
	}
}

func TestAttachmentMalformedResponsesAndCleanup(t *testing.T) {
	for _, response := range []string{
		`not-json`, `[]`, `{}`, `{"contentBase64":null}`, `{"contentBase64":12}`,
		`{"contentBase64":""}`, `{"contentBase64":"!!!!"}`, `{"contentBase64":"\n\n"}`,
		`{"path":"/must-not-read","reason":"too large"}`,
	} {
		t.Run(response, func(t *testing.T) {
			g := fakeGog(t, response)
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			t.Setenv("GUHD_HELPER_ATTACHMENT", "1")
			if _, err := g.Attachment(t.Context(), Account{}, "m", Attachment{ID: "a", MIMEType: "image/png", Size: 5}); err == nil {
				t.Fatal("invalid response accepted")
			}
			assertEmptyDirectory(t, dir)
		})
	}
}

func TestAttachmentDataLimits(t *testing.T) {
	for _, size := range []int{attachmentLimit, attachmentLimit + 1} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			g := fakeGog(t, "")
			response := `{"contentBase64":"` + base64.StdEncoding.EncodeToString(make([]byte, size)) + `"}`
			path := filepath.Join(t.TempDir(), "response.json")
			if err := os.WriteFile(path, []byte(response), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GUHD_HELPER_JSON_FILE", path)
			data, err := g.Attachment(t.Context(), Account{}, "m", Attachment{ID: "a", MIMEType: "image/png", Size: attachmentLimit})
			if size == attachmentLimit {
				if err != nil || len(data) != size {
					t.Fatalf("size=%d err=%v", len(data), err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "exceeds 3 MiB") {
				t.Fatalf("oversized data error=%v", err)
			}
		})
	}
}

func TestAttachmentCommandFailuresAndCleanup(t *testing.T) {
	for _, mode := range []string{"fail", "large", "wait", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			g := fakeGog(t, `{"contentBase64":"aGVsbG8="}`)
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			t.Setenv("GUHD_HELPER_ATTACHMENT", "1")
			t.Setenv("GUHD_HELPER_MODE", mode)
			ctx := t.Context()
			if mode == "wait" {
				g.Timeout = 50 * time.Millisecond
			}
			if mode == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := g.Attachment(ctx, Account{}, "m", Attachment{ID: "a", MIMEType: "image/png", Size: 5})
			if err == nil {
				t.Fatal("command failure accepted")
			}
			if mode == "wait" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			if mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			assertEmptyDirectory(t, dir)
		})
	}
}

func assertEmptyDirectory(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary directory entries=%v err=%v", entries, err)
	}
}

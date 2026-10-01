package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetailMIMETypes(t *testing.T) {
	for _, tc := range []struct{ name, payload, body, want string }{
		{"plain parameters", `{"mimeType":"multipart/alternative","parts":[{"mimeType":"text/plain; charset=utf-8","body":{"data":"encoded"}},{"mimeType":"text/html"}]}`, "Use <token> literally", "Use <token> literally"},
		{"plain case", `{"mimeType":"multipart/alternative","parts":[{"mimeType":" Text/Plain ","body":{"data":"encoded"}},{"mimeType":"text/html"}]}`, "Use <token> literally", "Use <token> literally"},
		{"html parameters", `{"mimeType":"text/html; charset=utf-8"}`, "<p>Hello &amp; welcome</p>", "Hello & welcome"},
		{"html case", `{"mimeType":" Text/HTML "}`, "<p>Hello</p>", "Hello"},
		{"invalid parameters fallback", `{"mimeType":"Text/HTML; broken"}`, "<p>Hello</p>", "Hello"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			g := fakeGog(t, `{"message":{"id":"m1","payload":`+tc.payload+`},"body":`+string(body)+`}`)
			detail, err := g.Detail(t.Context(), Account{}, "m1")
			if err != nil || detail.Body != tc.want {
				t.Fatalf("body=%q want=%q err=%v", detail.Body, tc.want, err)
			}
		})
	}
}

func TestDetailArguments(t *testing.T) {
	g := fakeGog(t, `{"message":{"id":"-opaque ID"}}`)
	path := filepath.Join(t.TempDir(), "args.json")
	t.Setenv("GUHD_HELPER_ARGV", path)
	account := Account{Email: "alex@example.com", Client: "work"}
	if _, err := g.Detail(t.Context(), account, "-opaque ID"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(data, &args); err != nil {
		t.Fatal(err)
	}
	want := []string{"--json", "--no-input", "--readonly", "--color", "never", "--wrap-untrusted=false", "--account", account.Email, "--client", account.Client, "gmail", "get", "--format", "full", "--use-indexed-attachment-ids=false", "--", "-opaque ID"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q want=%q", args, want)
	}
}

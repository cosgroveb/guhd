package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEventsOngoingDSTAndConflicts(t *testing.T) {
	g := fakeGog(t, `{"events":[
 {"id":"all","summary":"DST day","start":{"date":"2026-11-01"},"end":{"date":"2026-11-02"}},
 {"id":"ongoing","start":{"dateTime":"2026-11-01T00:00:00-04:00"},"end":{"dateTime":"2026-11-01T02:00:00-05:00"},"attendees":[{"email":"casey@example.net"}],"conferenceData":{"entryPoints":[{"uri":"https://meet.example.net/one"}]}},
 {"id":"overlap","start":{"dateTime":"2026-11-01T01:30:00-05:00"},"end":{"dateTime":"2026-11-01T02:30:00-05:00"}},
 {"id":"free","transparency":"transparent","start":{"dateTime":"2026-11-01T01:30:00-05:00"},"end":{"dateTime":"2026-11-01T02:30:00-05:00"}},
 {"id":"boundary","start":{"dateTime":"2026-11-01T02:30:00-05:00"},"end":{"dateTime":"2026-11-01T03:00:00-05:00"}},
 {"id":"ended","start":{"dateTime":"2026-10-31T23:00:00-04:00"},"end":{"dateTime":"2026-11-01T01:00:00-04:00"}},
 {"id":"cancelled","status":"cancelled"}
 ]}`)
	t.Setenv("GUHD_HELPER_CALENDARS", `{"calendars":[{"id":"opaque ID","timeZone":"America/New_York","summary":"Original","summaryOverride":"Work"}]}`)
	now, err := time.Parse(time.RFC3339, "2026-11-01T01:00:00-04:00")
	if err != nil {
		t.Fatal(err)
	}
	events, err := g.Events(t.Context(), Account{Email: "alex@example.com", Client: "work"}, []string{"opaque ID"}, now)
	if err != nil || len(events) != 5 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	byID := map[string]Event{}
	for _, event := range events {
		byID[event.ID] = event
	}
	if byID["all"].End.Sub(byID["all"].Start) != 25*time.Hour || byID["all"].Conflict || !byID["all"].AllDay {
		t.Fatalf("all-day=%+v", byID["all"])
	}
	if !byID["ongoing"].Conflict || !byID["overlap"].Conflict || byID["free"].Conflict || byID["boundary"].Conflict {
		t.Fatalf("conflicts=%+v", events)
	}
	if byID["ongoing"].Key() != "opaque ID\x00ongoing" || len(byID["ongoing"].MeetingLinks) != 1 {
		t.Fatal(byID["ongoing"])
	}
}

func TestCalendarMalformedResponses(t *testing.T) {
	g := fakeGog(t, `{"events":[{"id":"bad","start":{"date":"nonsense"},"end":{"date":"2026-11-02"}}]}`)
	t.Setenv("GUHD_HELPER_CALENDARS", `{"calendars":[{"id":"cal","timeZone":"UTC"}]}`)
	if _, err := g.Events(t.Context(), Account{}, []string{"cal"}, time.Now()); err == nil || !strings.Contains(err.Error(), "start") {
		t.Fatalf("error=%v", err)
	}
	t.Setenv("GUHD_HELPER_CALENDARS", `{"calendars":[{"id":"cal","timeZone":"Invalid/Zone"}]}`)
	if _, err := g.Events(t.Context(), Account{}, []string{"cal"}, time.Now()); err == nil || !strings.Contains(err.Error(), "timezone") {
		t.Fatalf("error=%v", err)
	}
	t.Setenv("GUHD_HELPER_CALENDARS", `{"calendars":[]}`)
	if _, err := g.Events(t.Context(), Account{}, []string{"cal"}, time.Now()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("error=%v", err)
	}
}

func TestCalendarFetchArguments(t *testing.T) {
	g := fakeGog(t, `{"events":[]}`)
	t.Setenv("GUHD_HELPER_CALENDARS", `{"calendars":[{"id":"opaque ID","timeZone":"America/New_York"},{"id":"-opaque/ID?#","timeZone":"UTC"}]}`)
	path := filepath.Join(t.TempDir(), "args.jsonl")
	t.Setenv("GUHD_HELPER_ARGV_LOG", path)
	now := time.Date(2026, 10, 31, 12, 34, 56, 0, time.FixedZone("east", 5*60*60+30*60))
	account := Account{Email: "alex@example.com", Client: "work"}
	ids := []string{"opaque ID", "-opaque/ID?#"}
	if _, err := g.Events(t.Context(), account, ids, now); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("calls=%d: %s", len(lines), data)
	}
	prefix := []string{"--json", "--no-input", "--readonly", "--color", "never", "--wrap-untrusted=false", "--account", account.Email, "--client", account.Client}
	for i, line := range lines {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatal(err)
		}
		command := []string{"calendar", "calendars", "--all"}
		if i > 0 {
			command = []string{"calendar", "events", "--from", "2026-10-31T12:34:56+05:30", "--to", "2026-11-30T12:34:56+05:30", "--max", "250", "--all-pages", "--", ids[i-1]}
		}
		want := append(append([]string(nil), prefix...), command...)
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("call %d args=%q want=%q", i, args, want)
		}
	}
}

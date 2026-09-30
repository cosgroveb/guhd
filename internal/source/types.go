package source

import "time"

type Account struct{ Email, Client, Error string }
type Calendar struct{ ID, Name, TimeZone string }
type Event struct {
	ID, CalendarID, Title, Description, Location, URL string
	Start, End                                        time.Time
	AllDay, Transparent, Conflict                     bool
	Attendees, MeetingLinks                           []string
}

func (e Event) Key() string { return e.CalendarID + "\x00" + e.ID }

type Message struct {
	ID, ThreadID, From, Subject, DateText, URL string
	Received                                   time.Time
	Unread                                     bool
}
type MailPage struct {
	Messages      []Message
	NextPageToken string
}
type MailCounts struct{ Total, Unread int }
type MessageDetail struct {
	Message  Message
	To, Body string
}
type Project struct {
	Name, Path, Subject, Body, Revision string
	Time                                time.Time
	Error                               string
}

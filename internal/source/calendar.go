package source

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func (g Gog) Calendars(ctx context.Context, account Account) ([]Calendar, error) {
	var response struct {
		Calendars []struct{ ID, Summary, SummaryOverride, TimeZone string }
	}
	if err := g.read(ctx, account, "calendars", &response, "calendar", "calendars", "--all"); err != nil {
		return nil, err
	}
	calendars := make([]Calendar, 0, len(response.Calendars))
	for _, c := range response.Calendars {
		name := c.Summary
		if c.SummaryOverride != "" {
			name = c.SummaryOverride
		}
		calendars = append(calendars, Calendar{ID: c.ID, Name: name, TimeZone: c.TimeZone})
	}
	return calendars, nil
}

type eventTime struct{ Date, DateTime string }
type wireEvent struct {
	ID, Summary, Description, Location, HTMLLink, HangoutLink, Status, Transparency string
	Start, End                                                                      eventTime
	Attendees                                                                       []struct{ Email, DisplayName, ResponseStatus string }
	ConferenceData                                                                  struct{ EntryPoints []struct{ URI string } }
}

func (g Gog) Events(ctx context.Context, account Account, calendarIDs []string, now time.Time) ([]Event, error) {
	if len(calendarIDs) == 0 {
		return nil, nil
	}
	calendars, err := g.Calendars(ctx, account)
	if err != nil {
		return nil, err
	}
	zones := make(map[string]string, len(calendars))
	for _, c := range calendars {
		zones[c.ID] = c.TimeZone
	}
	end := now.AddDate(0, 0, 30)
	var events []Event
	for _, id := range calendarIDs {
		zone, ok := zones[id]
		if !ok {
			return nil, fmt.Errorf("calendar %q is unavailable", id)
		}
		loc, err := time.LoadLocation(zone)
		if zone == "" || err != nil {
			return nil, fmt.Errorf("calendar %q has invalid timezone %q", id, zone)
		}
		var response struct{ Events []wireEvent }
		if err := g.read(ctx, account, "events", &response, "calendar", "events", "--from", now.Format(time.RFC3339), "--to", end.Format(time.RFC3339), "--max", "250", "--all-pages", "--", id); err != nil {
			return nil, err
		}
		for _, w := range response.Events {
			if w.Status == "cancelled" {
				continue
			}
			e := Event{ID: w.ID, CalendarID: id, Title: w.Summary, Description: w.Description, Location: w.Location, URL: w.HTMLLink, AllDay: w.Start.Date != "", Transparent: w.Transparency == "transparent"}
			e.Start, err = parseEventTime(w.Start, loc)
			if err != nil {
				return nil, fmt.Errorf("event %q start: %w", w.ID, err)
			}
			e.End, err = parseEventTime(w.End, loc)
			if err != nil {
				return nil, fmt.Errorf("event %q end: %w", w.ID, err)
			}
			if (w.Start.Date != "") != (w.End.Date != "") || !e.End.After(e.Start) {
				return nil, fmt.Errorf("event %q has invalid time interval", w.ID)
			}
			if !e.End.After(now) || !e.Start.Before(end) {
				continue
			}
			for _, a := range w.Attendees {
				label := a.Email
				if a.DisplayName != "" {
					label = a.DisplayName + " <" + a.Email + ">"
				}
				if a.ResponseStatus != "" {
					label += " (" + a.ResponseStatus + ")"
				}
				e.Attendees = append(e.Attendees, label)
			}
			if w.HangoutLink != "" {
				e.MeetingLinks = append(e.MeetingLinks, w.HangoutLink)
			}
			for _, p := range w.ConferenceData.EntryPoints {
				if p.URI != "" && p.URI != w.HangoutLink {
					e.MeetingLinks = append(e.MeetingLinks, p.URI)
				}
			}
			events = append(events, e)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Start.Before(events[j].Start) })
	for i := range events {
		if events[i].AllDay || events[i].Transparent {
			continue
		}
		for j := i + 1; j < len(events) && events[j].Start.Before(events[i].End); j++ {
			if !events[j].AllDay && !events[j].Transparent {
				events[i].Conflict = true
				events[j].Conflict = true
			}
		}
	}
	return events, nil
}

func parseEventTime(value eventTime, loc *time.Location) (time.Time, error) {
	if value.Date != "" {
		return time.ParseInLocation(time.DateOnly, value.Date, loc)
	}
	return time.Parse(time.RFC3339, value.DateTime)
}

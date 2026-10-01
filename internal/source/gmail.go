package source

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func mailURL(account, thread string) string {
	return "https://mail.google.com/mail/?authuser=" + url.QueryEscape(account) + "#all/" + url.PathEscape(thread)
}

func (g Gog) Messages(ctx context.Context, account Account, query, page string) (MailPage, error) {
	var response struct {
		Messages []struct {
			ID, ThreadID, From, Subject, Date, InternalDateISO string
			Labels                                             []string
		}
		NextPageToken string
	}
	args := []string{"gmail", "messages", "search", "--max", "50", "--timezone", "UTC"}
	if page != "" {
		args = append(args, "--page", page)
	}
	args = append(args, "--", query)
	if err := g.read(ctx, account, "messages", &response, args...); err != nil {
		return MailPage{}, err
	}
	result := MailPage{NextPageToken: response.NextPageToken}
	for _, w := range response.Messages {
		m := Message{ID: w.ID, ThreadID: w.ThreadID, From: w.From, Subject: w.Subject, DateText: w.Date, Unread: slices.Contains(w.Labels, "UNREAD"), URL: mailURL(account.Email, w.ThreadID)}
		if w.InternalDateISO != "" {
			var err error
			m.Received, err = time.Parse(time.RFC3339, w.InternalDateISO)
			if err != nil {
				return MailPage{}, fmt.Errorf("message %q receipt time: %w", w.ID, err)
			}
		}
		result.Messages = append(result.Messages, m)
	}
	return result, nil
}

func (g Gog) Counts(ctx context.Context, account Account) (MailCounts, error) {
	var response struct {
		Label *struct{ MessagesTotal, MessagesUnread int }
	}
	if err := g.read(ctx, account, "label", &response, "gmail", "labels", "get", "INBOX"); err != nil {
		return MailCounts{}, err
	}
	if response.Label == nil {
		return MailCounts{}, fmt.Errorf("gog returned null inbox label")
	}
	return MailCounts{Total: response.Label.MessagesTotal, Unread: response.Label.MessagesUnread}, nil
}

type mimePart struct {
	MIMEType string
	Parts    []mimePart
	Body     struct{ Data string }
}

func normalizeMIMEType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil && mediaType != "" {
		return strings.ToLower(mediaType)
	}

	if idx := strings.Index(value, ";"); idx != -1 {
		return strings.TrimSpace(value[:idx])
	}

	return value
}

func hasInlinePlain(part mimePart) bool {
	if normalizeMIMEType(part.MIMEType) == "text/plain" && part.Body.Data != "" {
		return true
	}
	for _, child := range part.Parts {
		if hasInlinePlain(child) {
			return true
		}
	}
	return false
}

func (g Gog) Detail(ctx context.Context, account Account, id string) (MessageDetail, error) {
	var response struct {
		Message *struct {
			ID, ThreadID, Snippet, InternalDate string
			LabelIDs                            []string
			Payload                             mimePart
		}
		Headers     struct{ From, To, Subject, Date string }
		Body        string
		Attachments []struct {
			AttachmentID, Filename, MIMEType string
			Size                             int
		}
	}
	if err := g.read(ctx, account, "message", &response, "gmail", "get", "--format", "full", "--use-indexed-attachment-ids=false", "--", id); err != nil {
		return MessageDetail{}, err
	}
	w := response.Message
	if w == nil {
		return MessageDetail{}, fmt.Errorf("gog returned null message")
	}
	m := Message{ID: w.ID, ThreadID: w.ThreadID, From: response.Headers.From, Subject: response.Headers.Subject, DateText: response.Headers.Date, Unread: slices.Contains(w.LabelIDs, "UNREAD"), URL: mailURL(account.Email, w.ThreadID)}
	if w.InternalDate != "" {
		millis, err := strconv.ParseInt(w.InternalDate, 10, 64)
		if err != nil {
			return MessageDetail{}, fmt.Errorf("message %q receipt time: %w", id, err)
		}
		m.Received = time.UnixMilli(millis)
	}
	body := response.Body
	if body == "" {
		body = w.Snippet
	} else if !hasInlinePlain(w.Payload) && hasHTML(w.Payload) {
		body = htmlText(body)
	}
	result := MessageDetail{Message: m, To: response.Headers.To, Body: body}
	for _, a := range response.Attachments {
		mediaType := normalizeMIMEType(a.MIMEType)
		if !strings.HasPrefix(mediaType, "image/") {
			continue
		}
		attachment := Attachment{ID: a.AttachmentID, Name: a.Filename, MIMEType: mediaType, Size: a.Size}
		attachment.Unavailable = attachmentUnavailable(attachment)
		result.Attachments = append(result.Attachments, attachment)
		if len(result.Attachments) == 20 {
			break
		}
	}
	return result, nil
}

func hasHTML(part mimePart) bool {
	if normalizeMIMEType(part.MIMEType) == "text/html" {
		return true
	}
	for _, child := range part.Parts {
		if hasHTML(child) {
			return true
		}
	}
	return false
}

func htmlText(body string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(body))
	var out strings.Builder
	hidden := 0
	for {
		switch tokenizer.Next() {
		case html.CommentToken, html.DoctypeToken:
			continue
		case html.ErrorToken:
			return strings.TrimSpace(out.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			tag, _ := tokenizer.TagName()
			name := string(tag)
			if name == "script" || name == "style" {
				hidden++
			}
			if hidden == 0 && (name == "br" || name == "p" || name == "div" || name == "li" || name == "tr") {
				out.WriteByte('\n')
			}
		case html.EndTagToken:
			tag, _ := tokenizer.TagName()
			name := string(tag)
			if (name == "script" || name == "style") && hidden > 0 {
				hidden--
			}
			if hidden == 0 && (name == "p" || name == "div" || name == "li" || name == "tr") {
				out.WriteByte('\n')
			}
		case html.TextToken:
			if hidden == 0 {
				out.Write(tokenizer.Text())
			}
		}
	}
}

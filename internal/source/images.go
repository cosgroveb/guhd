package source

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

const attachmentLimit = 3 << 20

func attachmentUnavailable(a Attachment) string {
	switch a.MIMEType {
	case "image/png", "image/jpeg", "image/gif":
	default:
		return "unsupported image type (PNG, JPEG and GIF only)"
	}
	if a.Size <= 0 {
		return "image size unavailable"
	}
	if a.Size > attachmentLimit {
		return "image exceeds 3 MiB limit"
	}
	if a.ID == "" {
		return "image attachment ID unavailable"
	}
	return ""
}

func (g Gog) Attachment(ctx context.Context, account Account, messageID string, attachment Attachment) ([]byte, error) {
	if reason := attachmentUnavailable(attachment); reason != "" {
		return nil, fmt.Errorf("attachment unavailable: %s", reason)
	}
	dir, err := os.MkdirTemp("", "guhd-image-*")
	if err != nil {
		return nil, fmt.Errorf("attachment temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	var response struct{ ContentBase64 string }
	if err := g.read(ctx, account, "contentBase64", &response,
		"gmail", "attachment", "--use-indexed-attachment-ids=false", "--inline",
		"--inline-max-bytes", "3145728", "--out", filepath.Join(dir, "attachment"), "--", messageID, attachment.ID); err != nil {
		return nil, err
	}
	if response.ContentBase64 == "" {
		return nil, fmt.Errorf("gog returned no inline attachment data")
	}
	if len(response.ContentBase64) > base64.StdEncoding.EncodedLen(attachmentLimit) {
		return nil, fmt.Errorf("attachment exceeds 3 MiB limit")
	}
	data, err := base64.StdEncoding.DecodeString(response.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid inline attachment data: %w", err)
	}
	if len(data) > attachmentLimit {
		return nil, fmt.Errorf("attachment exceeds 3 MiB limit")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("gog returned no inline attachment data")
	}
	return data, nil
}

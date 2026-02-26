package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type Client struct {
	svc *gmail.Service
}

type EmailData struct {
	ID      string
	From    string
	Subject string
	Body    string
}

type MessageHeader struct {
	Name  string
	Value string
}

func NewClient(ctx context.Context, ts oauth2.TokenSource) (*Client, error) {
	svc, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("creating gmail service: %w", err)
	}
	return &Client{svc: svc}, nil
}

// GetNewMessageIDs returns message IDs added to inbox since the given historyId.
// Returns the new historyId to use for the next poll.
func (c *Client) GetNewMessageIDs(ctx context.Context, historyID uint64) ([]struct{ ID, ThreadID string }, uint64, error) {
	resp, err := c.svc.Users.History.List("me").
		StartHistoryId(historyID).
		LabelId("INBOX").
		HistoryTypes("messageAdded").
		Context(ctx).
		Do()
	if err != nil {
		return nil, 0, fmt.Errorf("listing history: %w", err)
	}

	var messages []struct{ ID, ThreadID string }
	seen := make(map[string]bool)
	for _, h := range resp.History {
		for _, m := range h.MessagesAdded {
			if !seen[m.Message.Id] {
				seen[m.Message.Id] = true
				messages = append(messages, struct{ ID, ThreadID string }{
					ID:       m.Message.Id,
					ThreadID: m.Message.ThreadId,
				})
			}
		}
	}

	return messages, resp.HistoryId, nil
}

// GetEmail fetches an email and extracts relevant data for classification.
func (c *Client) GetEmail(ctx context.Context, messageID string) (*EmailData, error) {
	msg, err := c.svc.Users.Messages.Get("me", messageID).
		Format("full").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("getting message %s: %w", messageID, err)
	}

	var headers []MessageHeader
	for _, h := range msg.Payload.Headers {
		headers = append(headers, MessageHeader{Name: h.Name, Value: h.Value})
	}

	data := extractEmailHeaders(headers)
	data.ID = messageID
	data.Body = truncateText(extractBody(msg.Payload), 500)

	return data, nil
}

// ApplyLabel applies a Gmail label to a message.
func (c *Client) ApplyLabel(ctx context.Context, messageID, labelID string) error {
	_, err := c.svc.Users.Messages.Modify("me", messageID, &gmail.ModifyMessageRequest{
		AddLabelIds: []string{labelID},
	}).Context(ctx).Do()
	return err
}

// CreateLabel creates a Gmail label and returns its ID. If it already exists, returns the existing ID.
func (c *Client) CreateLabel(ctx context.Context, name string) (string, error) {
	label, err := c.svc.Users.Labels.Create("me", &gmail.Label{
		Name:                  name,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}).Context(ctx).Do()
	if err != nil {
		// Check if label already exists by listing all labels
		if existingID, findErr := c.findLabelByName(ctx, name); findErr == nil {
			return existingID, nil
		}
		return "", fmt.Errorf("creating label %q: %w", name, err)
	}
	return label.Id, nil
}

func (c *Client) findLabelByName(ctx context.Context, name string) (string, error) {
	resp, err := c.svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	for _, l := range resp.Labels {
		if l.Name == name {
			return l.Id, nil
		}
	}
	return "", fmt.Errorf("label %q not found", name)
}

// GetProfile returns the user's email and current historyId.
func (c *Client) GetProfile(ctx context.Context) (email string, historyID uint64, err error) {
	profile, err := c.svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return "", 0, err
	}
	return profile.EmailAddress, profile.HistoryId, nil
}

// ListRecentMessages returns recent message IDs from the inbox.
func (c *Client) ListRecentMessages(ctx context.Context, maxResults int64) ([]struct{ ID, ThreadID string }, error) {
	resp, err := c.svc.Users.Messages.List("me").
		LabelIds("INBOX").
		MaxResults(maxResults).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	var msgs []struct{ ID, ThreadID string }
	for _, m := range resp.Messages {
		msgs = append(msgs, struct{ ID, ThreadID string }{ID: m.Id, ThreadID: m.ThreadId})
	}
	return msgs, nil
}

func extractEmailHeaders(headers []MessageHeader) *EmailData {
	data := &EmailData{}
	for _, h := range headers {
		switch strings.ToLower(h.Name) {
		case "from":
			data.From = h.Value
		case "subject":
			data.Subject = h.Value
		}
	}
	return data
}

func extractBody(payload *gmail.MessagePart) string {
	// Try to find plain text part
	if payload.MimeType == "text/plain" && payload.Body != nil && payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(decoded)
		}
	}

	// Recurse into parts
	for _, part := range payload.Parts {
		if body := extractBody(part); body != "" {
			return body
		}
	}

	return ""
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

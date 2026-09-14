// Package discord sends report text to a Discord channel via an incoming
// webhook (no bot token or gateway connection required).
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// MessageLimit is Discord's per-message content character limit.
const MessageLimit = 2000

type Client struct {
	webhookURL string
	httpClient *http.Client
}

func NewClient(webhookURL string) *Client {
	return &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type payload struct {
	Content string `json:"content"`
}

// Send posts text to the configured webhook, splitting it into multiple
// messages if it exceeds Discord's character limit. Text is wrapped in a
// code block to preserve the fixed-width report formatting.
func (c *Client) Send(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, chunk := range splitMessage(text, MessageLimit-len("``````")) {
		if err := c.send(ctx, "```"+chunk+"```"); err != nil {
			return err
		}
	}
	return nil
}

// SendRich posts free-form Markdown text - e.g. the AI-generated weekly
// recap - without wrapping it in a code block, so Discord renders bold,
// italics, and headers instead of showing the literal characters.
func (c *Client) SendRich(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, chunk := range splitMessage(text, MessageLimit) {
		if err := c.send(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) send(ctx context.Context, content string) error {
	body, err := json.Marshal(payload{Content: content})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("discord: webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// SendImage posts a PNG image as a file attachment, with caption as the
// message content, via a multipart webhook request.
func (c *Client) SendImage(ctx context.Context, caption, filename string, data []byte) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	if caption != "" {
		if err := w.WriteField("content", caption); err != nil {
			return fmt.Errorf("discord: write caption field: %w", err)
		}
	}

	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("discord: create file part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("discord: write file part: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("discord: close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("discord: webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// splitMessage breaks text into chunks no longer than limit, preferring to
// break on line boundaries so a report's formatting stays intact.
func splitMessage(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder

	for _, line := range strings.Split(text, "\n") {
		if current.Len()+len(line)+1 > limit {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			// A single line longer than the limit gets hard-split.
			for len(line) > limit {
				chunks = append(chunks, line[:limit])
				line = line[limit:]
			}
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

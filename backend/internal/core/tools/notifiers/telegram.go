package notifiers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

type TelegramNotifier struct {
	Token  string
	ChatID string
	client *http.Client
}

func NewTelegramNotifier(token, chatID string, client *http.Client) *TelegramNotifier {
	return &TelegramNotifier{Token: token, ChatID: chatID, client: client}
}

// redactedToken replaces the bot token wherever an error would print it.
const redactedToken = "<redacted>"

// redact strips the bot token from a transport error. net/http's *url.Error
// embeds the full request URL, and the Telegram URL carries the token, so an
// unredacted error would leak it into logs and tool output. The wrapped cause
// is kept so errors.Is (context cancellation, timeouts) still works.
func (t *TelegramNotifier) redact(err error) error {
	var ue *url.Error
	if t.Token == "" || !errors.As(err, &ue) {
		return err
	}
	return &url.Error{Op: ue.Op, URL: strings.ReplaceAll(ue.URL, t.Token, redactedToken), Err: ue.Err}
}

func (t *TelegramNotifier) Name() string { return "Telegram" }

// telegramChunkSize is the largest part Send posts. Telegram rejects text over
// 4096 UTF-16 units; splitMessage counts in those units.
const telegramChunkSize = 4000

const (
	// telegramMaxRetryWait is the longest 429 wait worth blocking a run for.
	telegramMaxRetryWait = 10 * time.Second
	// telegramDefaultRetryWait applies when a 429 carries no retry_after.
	telegramDefaultRetryWait = time.Second
)

// Send delivers message, split on line boundaries when it exceeds Telegram's
// size limit. Markdown is tried first; if Telegram cannot parse the markup
// (model-written text often has stray `_` or `*`), the same part is re-sent as
// plain text rather than losing the message.
func (t *TelegramNotifier) Send(ctx context.Context, message string) error {
	if t.Token == "" || t.ChatID == "" || t.client == nil {
		return fmt.Errorf("telegram connector not fully configured")
	}
	for _, part := range splitMessage(message, telegramChunkSize) {
		if err := t.sendPart(ctx, part); err != nil {
			return err
		}
	}
	return nil
}

func (t *TelegramNotifier) sendPart(ctx context.Context, text string) error {
	status, body, err := t.postRateLimited(ctx, text, "Markdown")
	if err == nil && status == http.StatusBadRequest && strings.Contains(body, "can't parse entities") {
		status, body, err = t.postRateLimited(ctx, text, "")
	}
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	err = fmt.Errorf("telegram API error: status %d, body: %s", status, body)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		// Rejected bot token — operator-actionable, so classify terminal for
		// the agent loop (see tool-error-classification).
		return fmt.Errorf("%w: %v", models.ErrToolUnavailable, err)
	}
	return err
}

// postRateLimited is post with one retry when Telegram answers 429 and asks for
// a wait within telegramMaxRetryWait, so a burst of chunks does not drop the rest
// of a digest. A longer wait is returned as the 429 error rather than blocking
// the run (which holds its lane slot while delivering).
func (t *TelegramNotifier) postRateLimited(ctx context.Context, text, parseMode string) (int, string, error) {
	status, body, err := t.post(ctx, text, parseMode)
	if err != nil || status != http.StatusTooManyRequests {
		return status, body, err
	}
	wait, ok := retryAfter(body)
	if !ok {
		return status, body, err
	}
	select {
	case <-ctx.Done():
		return 0, "", ctx.Err()
	case <-time.After(wait):
	}
	return t.post(ctx, text, parseMode)
}

// retryAfter reads Telegram's retry_after (seconds) from a 429 body; ok is
// false when the wait exceeds telegramMaxRetryWait.
func retryAfter(body string) (time.Duration, bool) {
	var resp struct {
		Parameters struct {
			RetryAfter *int `json:"retry_after"`
		} `json:"parameters"`
	}
	wait := telegramDefaultRetryWait
	if json.Unmarshal([]byte(body), &resp) == nil && resp.Parameters.RetryAfter != nil && *resp.Parameters.RetryAfter >= 0 {
		wait = time.Duration(*resp.Parameters.RetryAfter) * time.Second
	}
	return wait, wait <= telegramMaxRetryWait
}

// post performs one sendMessage call; an empty parseMode sends plain text. The
// response body is read (capped) only for a non-200 status.
func (t *TelegramNotifier) post(ctx context.Context, text, parseMode string) (int, string, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	formData := url.Values{}
	formData.Set("chat_id", t.ChatID)
	formData.Set("text", text)
	if parseMode != "" {
		formData.Set("parse_mode", parseMode)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return 0, "", t.redact(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, "", t.redact(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return resp.StatusCode, "", nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return resp.StatusCode, string(body), nil
}

// utf16Len is the length Telegram enforces: UTF-16 code units, so a character
// outside the Basic Multilingual Plane (most emoji) counts as two.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// hardSplit cuts one line into pieces of at most limit UTF-16 units; a line
// that already fits is returned whole.
func hardSplit(line string, limit int) []string {
	if utf16Len(line) <= limit {
		return []string{line}
	}
	var pieces []string
	start, units := 0, 0
	for i, r := range line {
		w := utf16.RuneLen(r)
		if units+w > limit {
			pieces = append(pieces, line[start:i])
			start, units = i, 0
		}
		units += w
	}
	return append(pieces, line[start:])
}

// splitMessage cuts text into parts of at most limit UTF-16 units, breaking on
// line boundaries; a single line longer than limit is hard-split.
func splitMessage(text string, limit int) []string {
	if utf16Len(text) <= limit {
		return []string{text}
	}
	var parts []string
	var cur []string
	curLen := 0
	flush := func() {
		if len(cur) > 0 {
			parts = append(parts, strings.Join(cur, "\n"))
			cur, curLen = nil, 0
		}
	}
	for _, line := range strings.Split(text, "\n") {
		for _, seg := range hardSplit(line, limit) {
			add := utf16Len(seg)
			if len(cur) > 0 {
				add++ // joining newline
			}
			if curLen+add > limit {
				flush()
				add = utf16Len(seg)
			}
			cur = append(cur, seg)
			curLen += add
		}
	}
	flush()
	return parts
}

type WebhookInfo struct {
	URL            string `json:"url"`
	PendingUpdates int    `json:"pending_updates"`
	LastError      string `json:"last_error,omitempty"`
}

func (t *TelegramNotifier) RegisterWebhook(ctx context.Context, webhookURL, secretToken string) error {
	if t.Token == "" || t.client == nil {
		return fmt.Errorf("telegram connector not fully configured")
	}

	formData := url.Values{}
	formData.Set("url", webhookURL)
	formData.Set("allowed_updates", `["message"]`)
	formData.Set("drop_pending_updates", "true")
	if secretToken != "" {
		formData.Set("secret_token", secretToken)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", t.Token)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram setWebhook request failed: %w", t.redact(err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("telegram setWebhook parse error: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram setWebhook rejected: %s", result.Description)
	}
	return nil
}

func (t *TelegramNotifier) GetWebhookInfo(ctx context.Context) (*WebhookInfo, error) {
	if t.Token == "" || t.client == nil {
		return nil, fmt.Errorf("telegram connector not fully configured")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getWebhookInfo", t.Token)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram getWebhookInfo failed: %w", t.redact(err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var tgResp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			URL            string `json:"url"`
			PendingUpdates int    `json:"pending_update_count"`
			LastErrorDate  int64  `json:"last_error_date,omitempty"`
			LastErrorMsg   string `json:"last_error_message,omitempty"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &tgResp); err != nil {
		return nil, fmt.Errorf("telegram getWebhookInfo parse error: %w", err)
	}
	if !tgResp.OK {
		return nil, fmt.Errorf("telegram getWebhookInfo rejected: %s", tgResp.Description)
	}
	return &WebhookInfo{
		URL:            tgResp.Result.URL,
		PendingUpdates: tgResp.Result.PendingUpdates,
		LastError:      tgResp.Result.LastErrorMsg,
	}, nil
}

func (t *TelegramNotifier) DeleteWebhook(ctx context.Context) error {
	if t.Token == "" || t.client == nil {
		return fmt.Errorf("telegram connector not fully configured")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook", t.Token)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram deleteWebhook failed: %w", t.redact(err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("telegram deleteWebhook parse error: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram deleteWebhook rejected: %s", result.Description)
	}
	return nil
}

// init registers the Telegram connector with the dynamic connector registry so
// that initCommunicationTools can build it by type string without a hardcoded
// switch. Adding another platform means writing a similar init() in its own
// package — no wiring-layer changes required.
func init() {
	tools.RegisterConnectorFactory(models.ConnectorTypeTelegram, func(
		name string,
		cfg models.ConnectorConfig,
		secrets models.SecretsStore,
		network *tools.NetworkTools,
	) (tools.Connector, bool) {
		token := secrets.GetSecret("connector", name)
		chatID := cfg.Settings["chat_id"]
		if token == "" || chatID == "" {
			return nil, false
		}
		return NewTelegramNotifier(token, chatID, network.HTTPClient()), true
	})
}

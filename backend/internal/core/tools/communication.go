package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Notifier defines the interface for sending messages to various platforms.
type Notifier interface {
	Send(ctx context.Context, message string) error
	Name() string
}

// CommunicationTools manages multiple notification providers.
type CommunicationTools struct {
	notifiers []Notifier
}

func NewCommunicationTools() *CommunicationTools {
	return &CommunicationTools{}
}

func (c *CommunicationTools) AddNotifier(n Notifier) {
	c.notifiers = append(c.notifiers, n)
}

// NotifyAll sends a message to all configured notifiers.
func (c *CommunicationTools) NotifyAll(ctx context.Context, message string) error {
	var errs []error
	for _, n := range c.notifiers {
		if err := n.Send(ctx, message); err != nil {
			errs = append(errs, fmt.Errorf("%s failed: %w", n.Name(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("some notifications failed: %v", errs)
	}
	return nil
}

// TelegramNotifier implements the Notifier interface for Telegram.
type TelegramNotifier struct {
	Token  string
	ChatID string
}

func (t *TelegramNotifier) Name() string { return "Telegram" }

func (t *TelegramNotifier) Send(ctx context.Context, message string) error {
	if t.Token == "" || t.ChatID == "" {
		return fmt.Errorf("telegram credentials missing")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	
	formData := url.Values{}
	formData.Set("chat_id", t.ChatID)
	formData.Set("text", message)
	formData.Set("parse_mode", "Markdown")

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = formData.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API error: status %d", resp.StatusCode)
	}

	return nil
}

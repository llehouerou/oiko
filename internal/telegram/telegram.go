// Package telegram sends Oiko's Notifications to one chat through a Telegram
// bot.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/llehouerou/oiko/internal/automation"
)

// Config is the "telegram" section of Oiko's configuration. Both values are
// read from files, so that they stay out of it.
type Config struct {
	TokenFile  string `json:"tokenFile"`
	ChatIDFile string `json:"chatIdFile"`
}

const (
	sendTimeout = 10 * time.Second
	// ponytail: Notifications beyond this many waiting are dropped, logged;
	// a few per day are expected.
	queued = 64
)

// Bot sends Notifications, one at a time and in order, off the automation
// engine's goroutine. One that fails is logged, never retried.
type Bot struct {
	api    string // the Bot API's base URL, with the token
	chatID string
	queue  chan automation.Notification
}

func New(c Config) (*Bot, error) {
	token, err := readSecret(c.TokenFile)
	if err != nil {
		return nil, err
	}
	chatID, err := readSecret(c.ChatIDFile)
	if err != nil {
		return nil, err
	}
	return &Bot{api: "https://api.telegram.org/bot" + token, chatID: chatID, queue: make(chan automation.Notification, queued)}, nil
}

func readSecret(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("telegram: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// Notify queues n and returns at once.
func (b *Bot) Notify(n automation.Notification) {
	select {
	case b.queue <- n:
	default:
		log.Printf("telegram: %d notifications waiting, dropping %q", queued, n.Title)
	}
}

// Run sends queued Notifications until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-b.queue:
			if err := b.send(ctx, n); err != nil {
				log.Printf("telegram: sending %q: %v", n.Title, err)
			}
		}
	}
}

func (b *Bot) send(ctx context.Context, n automation.Notification) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	text := strings.TrimSpace(n.Title + "\n" + n.Message)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.api+"/sendMessage",
		strings.NewReader(url.Values{"chat_id": {b.chatID}, "text": {text}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if ue := (*url.Error)(nil); errors.As(err, &ue) {
		return ue.Err // its URL holds the token
	} else if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 200))
		return fmt.Errorf("%s: %s", res.Status, body)
	}
	return nil
}

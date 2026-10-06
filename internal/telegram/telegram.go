// Package telegram sends Oiko's Notifications to one chat through a Telegram
// bot, with the video of the Recording one carries (ADR 0039).
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/home"
)

// Config is the "telegram" section of Oiko's configuration. Both values are
// read from files, so that they stay out of it.
type Config struct {
	TokenFile  string `json:"tokenFile"`
	ChatIDFile string `json:"chatIdFile"`
}

// Videos is where the video of the Recording a Notification carries is
// found: the home's cameras.
type Videos interface {
	// Video fetches the video of the Recording of camera t that started at.
	Video(ctx context.Context, t home.Target, at time.Time) (*http.Response, error)
}

const (
	sendTimeout  = 10 * time.Second
	videoTimeout = 3 * time.Minute
	// maxVideo is the most a bot may upload (Bot API).
	maxVideo = 50 << 20
	// maxCaption is the longest caption Telegram takes, in characters.
	maxCaption = 1024
	// ponytail: Notifications beyond this many waiting are dropped, logged;
	// a few per day are expected.
	queued = 64
)

// Bot sends Notifications, one at a time and in order, off the automation
// engine's goroutine. One that fails is logged, never retried; one whose
// video cannot be sent goes as text.
type Bot struct {
	api    string // the Bot API's base URL, with the token
	chatID string
	videos Videos
	queue  chan automation.Notification
}

func New(c Config, videos Videos) (*Bot, error) {
	token, err := readSecret(c.TokenFile)
	if err != nil {
		return nil, err
	}
	chatID, err := readSecret(c.ChatIDFile)
	if err != nil {
		return nil, err
	}
	return &Bot{api: "https://api.telegram.org/bot" + token, chatID: chatID, videos: videos,
		queue: make(chan automation.Notification, queued)}, nil
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

// send sends n, as the video of its Recording if it carries one and that
// goes through, else as text.
func (b *Bot) send(ctx context.Context, n automation.Notification) error {
	text := strings.TrimSpace(n.Title + "\n" + n.Message)
	if n.Recording != nil {
		err := b.sendVideo(ctx, *n.Recording, text)
		if err == nil {
			return nil
		}
		log.Printf("telegram: the video of %q: %v", n.Title, err)
		text = strings.TrimSpace(text + "\n(its video could not be sent)")
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.api+"/sendMessage",
		strings.NewReader(url.Values{"chat_id": {b.chatID}, "text": {text}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(req)
}

// sendVideo uploads the video of Recording r with caption as it reads it,
// without keeping it.
func (b *Bot) sendVideo(ctx context.Context, r automation.RecordingAt, caption string) error {
	if b.videos == nil {
		return errors.New("no Recordings to read")
	}
	ctx, cancel := context.WithTimeout(ctx, videoTimeout)
	defer cancel()
	video, err := b.videos.Video(ctx, r.Camera, r.Start)
	if err != nil {
		return err
	}
	defer video.Body.Close()
	switch {
	case video.StatusCode != http.StatusOK:
		return fmt.Errorf("its video: %s", video.Status)
	case video.ContentLength > maxVideo:
		return fmt.Errorf("its video is %d MB, over the %d MB a bot may send", video.ContentLength>>20, maxVideo>>20)
	}
	if c := []rune(caption); len(c) > maxCaption {
		caption = string(c[:maxCaption-1]) + "…"
	}
	body, form := io.Pipe()
	w := multipart.NewWriter(form)
	go func() {
		for _, f := range [][2]string{{"chat_id", b.chatID}, {"caption", caption}, {"supports_streaming", "true"}} {
			w.WriteField(f[0], f[1])
		}
		part, err := w.CreateFormFile("video", "recording.mp4")
		if err == nil {
			var n int64
			if n, err = io.Copy(part, io.LimitReader(video.Body, maxVideo+1)); err == nil && n > maxVideo {
				err = fmt.Errorf("its video is over the %d MB a bot may send", maxVideo>>20)
			}
		}
		if err == nil {
			err = w.Close()
		}
		form.CloseWithError(err)
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.api+"/sendVideo", body)
	if err != nil {
		body.Close()
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	err = do(req)
	body.Close() // ends the copy if Telegram answered before reading it all
	return err
}

// do sends req to the Bot API, whose answer must be 200.
func do(req *http.Request) error {
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

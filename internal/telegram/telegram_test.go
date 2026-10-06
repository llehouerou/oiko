package telegram

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/home"
)

func TestSendPostsToTheChatAndHidesTheToken(t *testing.T) {
	var path, chat, text string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, chat, text = r.URL.Path, r.FormValue("chat_id"), r.FormValue("text")
	}))
	b := &Bot{api: srv.URL + "/botSECRET", chatID: "42"}
	if err := b.send(context.Background(), automation.Notification{Title: "🚨 Intrusion alert", Message: "Front door"}); err != nil {
		t.Fatal(err)
	}
	if path != "/botSECRET/sendMessage" || chat != "42" || text != "🚨 Intrusion alert\nFront door" {
		t.Errorf("posted %s chat %s: %q", path, chat, text)
	}

	srv.Close()
	err := b.send(context.Background(), automation.Notification{Title: "x"})
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("unreachable: %v", err)
	}
}

// videos serve the video of every Recording, size bytes long if size is
// set, told by their Content-Length, or else "mp4 of <camera> at <time>";
// unless err.
type videos struct {
	err  error
	size int64
}

var start = time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)

func (v videos) Video(_ context.Context, t home.Target, at time.Time) (*http.Response, error) {
	switch {
	case v.err != nil:
		return nil, v.err
	case v.size > 0:
		return &http.Response{StatusCode: http.StatusOK, ContentLength: v.size, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}
	body := "mp4 of " + t.Key() + " at " + at.Format(time.RFC3339)
	return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(strings.NewReader(body))}, nil
}

// telegram records what each method of the Bot API was sent.
func telegram(t *testing.T) (*httptest.Server, map[string]map[string]string) {
	got := map[string]map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := map[string]string{}
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			for k, v := range r.MultipartForm.Value {
				fields[k] = v[0]
			}
			if f, h, err := r.FormFile("video"); err == nil {
				b, _ := io.ReadAll(f)
				fields["video"], fields["filename"] = string(b), h.Filename
			}
		} else {
			r.ParseForm()
			for k, v := range r.PostForm {
				fields[k] = v[0]
			}
		}
		got[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]] = fields
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestANotificationWithARecordingIsSentAsItsVideo(t *testing.T) {
	srv, got := telegram(t)
	b := &Bot{api: srv.URL + "/botSECRET", chatID: "42", videos: videos{}}
	garden := home.TargetDevice("garden", "camera")
	n := automation.Notification{Title: "Garden", Message: "Someone", Recording: &automation.RecordingAt{Camera: garden, Start: start}}
	if err := b.send(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	v := got["sendVideo"]
	if v["chat_id"] != "42" || v["caption"] != "Garden\nSomeone" || v["video"] != "mp4 of "+garden.Key()+" at 2026-10-01T08:30:00Z" ||
		v["filename"] != "recording.mp4" || got["sendMessage"] != nil {
		t.Errorf("sent %v", got)
	}
}

func TestANotificationWhoseVideoFailsIsSentAsText(t *testing.T) {
	for name, b := range map[string]*Bot{
		"a failing fetch":      {videos: videos{err: errors.New("unreachable")}},
		"no Recordings":        {},
		"over the bot's limit": {videos: videos{size: maxVideo + 1}},
	} {
		srv, got := telegram(t)
		b.api, b.chatID = srv.URL+"/botSECRET", "42"
		n := automation.Notification{Title: "Garden", Recording: &automation.RecordingAt{Camera: home.TargetDevice("garden", "camera"), Start: start}}
		if err := b.send(context.Background(), n); err != nil {
			t.Fatal(err)
		}
		if got["sendVideo"] != nil || got["sendMessage"]["text"] != "Garden\n(its video could not be sent)" {
			t.Errorf("%s: sent %v", name, got)
		}
	}
}

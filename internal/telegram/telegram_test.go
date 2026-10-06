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

	"github.com/llehouerou/oiko/bridge"
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

// camera keeps two Recordings, a minute apart, whose videos are their ids.
type camera struct{ err error }

var start = time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)

func (c camera) Recordings(_ context.Context, _ home.Target, from, to time.Time) ([]bridge.Recording, error) {
	var rs []bridge.Recording
	for _, r := range []bridge.Recording{{ID: "early", Start: start.Add(-time.Minute)}, {ID: "this", Start: start.Add(time.Second)}} {
		if !r.Start.Before(from) && !r.Start.After(to) {
			rs = append(rs, r)
		}
	}
	return rs, nil
}

func (c camera) RecordingMedia(_ context.Context, _ home.Target, id string, part bridge.RecordingPart, _ http.Header) (*http.Response, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(strings.NewReader("mp4 of " + id + " " + string(part)))}, nil
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
	b := &Bot{api: srv.URL + "/botSECRET", chatID: "42", recordings: camera{}}
	n := automation.Notification{Title: "Garden", Message: "Someone", Recording: &automation.RecordingAt{Camera: home.TargetDevice("garden", "camera"), Start: start}}
	if err := b.send(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	v := got["sendVideo"]
	if v["chat_id"] != "42" || v["caption"] != "Garden\nSomeone" || v["video"] != "mp4 of this video" || v["filename"] != "recording.mp4" || got["sendMessage"] != nil {
		t.Errorf("sent %v", got)
	}
}

func TestANotificationWhoseVideoFailsIsSentAsText(t *testing.T) {
	for name, b := range map[string]*Bot{
		"a failing fetch": {recordings: camera{err: errors.New("unreachable")}},
		"no Recordings":   {},
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
	// None started at its time: the minute-old one is not taken for it.
	srv, got := telegram(t)
	b := &Bot{api: srv.URL + "/botSECRET", chatID: "42", recordings: camera{}}
	n := automation.Notification{Title: "Garden", Recording: &automation.RecordingAt{Camera: home.TargetDevice("garden", "camera"), Start: start.Add(-30 * time.Second)}}
	b.send(context.Background(), n)
	if got["sendVideo"] != nil || got["sendMessage"] == nil {
		t.Errorf("no Recording at its time: sent %v", got)
	}
}

func TestAVideoOverTheBotsLimitIsSentAsText(t *testing.T) {
	srv, got := telegram(t)
	big := camera{}
	b := &Bot{api: srv.URL + "/botSECRET", chatID: "42", recordings: sized{big, maxVideo + 1}}
	n := automation.Notification{Title: "Garden", Recording: &automation.RecordingAt{Camera: home.TargetDevice("garden", "camera"), Start: start}}
	if err := b.send(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if got["sendVideo"] != nil || got["sendMessage"] == nil {
		t.Errorf("sent %v", got)
	}
}

// sized is a camera whose videos are n bytes, told by their Content-Length.
type sized struct {
	camera
	n int64
}

func (s sized) RecordingMedia(context.Context, home.Target, string, bridge.RecordingPart, http.Header) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, ContentLength: s.n, Body: io.NopCloser(bytes.NewReader(nil))}, nil
}

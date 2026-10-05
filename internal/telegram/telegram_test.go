package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/internal/automation"
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

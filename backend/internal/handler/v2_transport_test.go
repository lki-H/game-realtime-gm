package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestV2ClientClosesWhenSendQueueExhausted(t *testing.T) {
	result := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(writer, request, nil)
		if err != nil {
			result <- false
			return
		}
		defer connection.Close()
		client := &v2Client{conn: connection, send: make(chan any, 1)}
		result <- client.enqueue("first") && !client.enqueue("second")
	}))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	select {
	case ok := <-result:
		if !ok {
			t.Fatal("full send queue did not disconnect")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queue handling blocked")
	}
	connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("exhausted connection remained open")
	}
}

func TestV2NotificationBackpressureDoesNotBlockOtherPlayers(t *testing.T) {
	result := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(writer, request, nil)
		if err != nil {
			result <- false
			return
		}
		defer connection.Close()
		slow := &v2Client{conn: connection, send: make(chan any, 1)}
		slow.send <- "unread"
		healthy := &v2Client{send: make(chan any, 2)}
		transport := &V2Transport{ctx: context.Background(), clients: map[int64]*v2Client{1: slow, 2: healthy}}
		started := time.Now()
		transport.Notify([]int64{1, 2}, "v2.test", []byte(`{}`))
		result <- time.Since(started) < time.Second && len(healthy.send) == 1
	}))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	select {
	case ok := <-result:
		if !ok {
			t.Fatal("slow player blocked notification to healthy player")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("notification blocked")
	}
}

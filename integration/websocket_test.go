//go:build integration

package censorshipintegration

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocketModelTurnUsesResponsesSelector(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: strip\nignore_case: true\nwords: [Alpha]\n")
	conn := dialResponsesWebSocket(t, cpa.wsURL, downstreamKey)
	t.Cleanup(func() { _ = conn.Close() })

	payload := []byte(`{"type":"response.create","model":"censorship-integration-model","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"aLPHA websocket"}]}]}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	messages := readUntilCompleted(t, conn)
	if len(messages) == 0 {
		t.Fatal("no websocket completion messages")
	}
	captured := upstream.lastRequest()
	if got := gjson.GetBytes(captured, "messages.0.content.0.text").String(); got != " websocket" {
		t.Fatalf("upstream input text = %q, body = %s", got, captured)
	}
}

const websocketCompletionReadTimeout = 20 * time.Second

type wsMessage struct {
	Opcode  int
	Payload []byte
}

func TestReadUntilCompletedReturnsOnDeadline(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	_, err = readUntilCompletedWithTimeout(conn, 10*time.Millisecond)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("error = %v, want read timeout", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout helper took %v", time.Since(start))
	}
}

func TestResponsesWebSocketBlockReturnsStatus400ThenCloses(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [SECRET]\n")
	conn := dialResponsesWebSocket(t, cpa.wsURL, downstreamKey)
	t.Cleanup(func() { _ = conn.Close() })

	payload := []byte(`{"type":"response.create","model":"censorship-integration-model","input":"SECRET"}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	opcode, event, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if opcode != websocket.TextMessage || gjson.GetBytes(event, "status").Int() != 400 {
		t.Fatalf("opcode=%d event=%s", opcode, event)
	}
	if gjson.GetBytes(event, "error.term").Exists() || gjson.GetBytes(event, "error.role").Exists() {
		t.Fatalf("WebSocket unexpectedly retained plugin direct body: %s", event)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("connection stayed open after terminal 400 event")
	}
	if upstream.requestCount() != 0 {
		t.Fatal("blocked WebSocket turn reached upstream")
	}
}

func TestResponsesWebSocketOutputMessagesUnaffected(t *testing.T) {
	payload := []byte(`{"type":"response.create","model":"censorship-integration-model","input":"plain input"}`)
	disabledUpstream := newMockUpstream(t)
	enabledUpstream := newMockUpstream(t)
	disabled := startCPA(t, disabledUpstream.URL, false, "")
	enabled := startCPA(t, enabledUpstream.URL, true, "mode: strip\nwords: [NEVER-MATCH]\n")
	gotDisabled := responsesWSExchange(t, disabled, payload)
	gotEnabled := responsesWSExchange(t, enabled, payload)
	if !reflect.DeepEqual(gotEnabled, gotDisabled) {
		t.Fatalf("enabled messages = %#v, disabled messages = %#v", gotEnabled, gotDisabled)
	}

	for _, tc := range []struct {
		name, config, wantInput string
	}{
		{name: "strip", config: "mode: strip\nwords: [SECRET]\n", wantInput: " input"},
		{name: "obfs", config: "mode: obfs\nwords: [SECRET]\n", wantInput: "S​ECRET input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transformedUpstream := newMockUpstream(t)
			baselineUpstream := newMockUpstream(t)
			transformed := startCPA(t, transformedUpstream.URL, true, tc.config)
			baseline := startCPA(t, baselineUpstream.URL, false, "")
			matching := []byte(`{"type":"response.create","model":"censorship-integration-model","input":"SECRET input"}`)
			gotTransformed := responsesWSExchange(t, transformed, matching)
			gotBaseline := responsesWSExchange(t, baseline, matching)
			captured := transformedUpstream.lastRequest()
			if got := gjson.GetBytes(captured, "messages.0.content").String(); got != tc.wantInput {
				t.Fatalf("upstream input = %q, want %q, body = %s", got, tc.wantInput, captured)
			}
			if !reflect.DeepEqual(gotTransformed, gotBaseline) {
				t.Fatalf("transformed messages = %#v, baseline messages = %#v", gotTransformed, gotBaseline)
			}
		})
	}
}

func responsesWSExchange(t *testing.T, cpa *cpaInstance, payload []byte) []wsMessage {
	t.Helper()

	conn := dialResponsesWebSocket(t, cpa.wsURL, downstreamKey)
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	return readUntilCompleted(t, conn)
}

func readUntilCompleted(t *testing.T, conn *websocket.Conn) []wsMessage {
	t.Helper()
	messages, err := readUntilCompletedWithTimeout(conn, websocketCompletionReadTimeout)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

func readUntilCompletedWithTimeout(conn *websocket.Conn, timeout time.Duration) ([]wsMessage, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	defer conn.SetReadDeadline(time.Time{})
	var messages []wsMessage
	for {
		opcode, payload, err := conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		messages = append(messages, wsMessage{Opcode: opcode, Payload: bytes.Clone(payload)})
		if gjson.GetBytes(payload, "type").String() == "response.completed" {
			return messages, nil
		}
	}
}

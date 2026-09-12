//go:build integration

package censorshipintegration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const websocketCompletionReadTimeout = integrationIOTimeout

func TestResponsesWebSocketDialTimesOutDuringStalledUpgrade(t *testing.T) {
	const timeout = 100 * time.Millisecond
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	startedAt := time.Now()
	conn, err := dialResponsesWebSocketWithTimeout("ws"+strings.TrimPrefix(server.URL, "http"), downstreamKey, timeout)
	if conn != nil {
		_ = conn.Close()
		t.Fatal("stalled upgrade unexpectedly connected")
	}
	if err == nil {
		t.Fatal("stalled upgrade unexpectedly completed")
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("WebSocket dial elapsed = %v, want <= %v", elapsed, time.Second)
	}
	select {
	case <-started:
	default:
		t.Fatal("stalled upgrade did not reach server")
	}
}

type responsesWebSocketEvent struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Error  struct {
		Term json.RawMessage `json:"term"`
		Role json.RawMessage `json:"role"`
	} `json:"error"`
}

func TestDecodeResponsesWebSocketEvent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		opcode     int
		payload    string
		wantType   string
		wantStatus int
		wantErr    bool
	}{
		{name: "valid text", opcode: websocket.TextMessage, payload: `{"type":"response.completed","status":400}`, wantType: "response.completed", wantStatus: 400},
		{name: "truncated", opcode: websocket.TextMessage, payload: `{"status":400`, wantErr: true},
		{name: "string status", opcode: websocket.TextMessage, payload: `{"status":"400"}`, wantErr: true},
		{name: "fractional status", opcode: websocket.TextMessage, payload: `{"status":400.9}`, wantErr: true},
		{name: "binary JSON", opcode: websocket.BinaryMessage, payload: `{"status":400}`, wantErr: true},
		{name: "top-level null", opcode: websocket.TextMessage, payload: `null`, wantErr: true},
		{name: "null type", opcode: websocket.TextMessage, payload: `{"type":null}`, wantErr: true},
		{name: "null status", opcode: websocket.TextMessage, payload: `{"status":null}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := decodeResponsesWebSocketEvent(tc.opcode, []byte(tc.payload))
			if tc.wantErr {
				if err == nil {
					t.Fatal("decode error = nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if event.Type != tc.wantType || event.Status != tc.wantStatus {
				t.Fatalf("event = %+v, want type=%q status=%d", event, tc.wantType, tc.wantStatus)
			}
		})
	}
}

func decodeResponsesWebSocketEvent(opcode int, payload []byte) (responsesWebSocketEvent, error) {
	if opcode != websocket.TextMessage {
		return responsesWebSocketEvent{}, fmt.Errorf("unexpected WebSocket message opcode %d", opcode)
	}
	var raw struct {
		Type   json.RawMessage `json:"type"`
		Status json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return responsesWebSocketEvent{}, fmt.Errorf("decode Responses WebSocket event: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		return responsesWebSocketEvent{}, fmt.Errorf("decode Responses WebSocket event: top-level null")
	}
	if len(raw.Type) > 0 && bytes.Equal(bytes.TrimSpace(raw.Type), []byte("null")) {
		return responsesWebSocketEvent{}, fmt.Errorf("decode Responses WebSocket event: type is null")
	}
	if len(raw.Status) > 0 && bytes.Equal(bytes.TrimSpace(raw.Status), []byte("null")) {
		return responsesWebSocketEvent{}, fmt.Errorf("decode Responses WebSocket event: status is null")
	}
	var event responsesWebSocketEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return responsesWebSocketEvent{}, fmt.Errorf("decode Responses WebSocket event: %w", err)
	}
	return event, nil
}

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

func TestResponsesWebSocketReadTimesOutAfterDial(t *testing.T) {
	const timeout = 100 * time.Millisecond
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

	conn, err := dialResponsesWebSocketWithTimeout("ws"+strings.TrimPrefix(server.URL, "http"), downstreamKey, timeout)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	started := time.Now()
	_, _, err = conn.ReadMessage()
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("WebSocket timeout elapsed = %v", elapsed)
	}
}

func TestTerminalWebSocketTimeoutIsNotPeerClose(t *testing.T) {
	const timeout = 100 * time.Millisecond
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"status":400}`)); err != nil {
			return
		}
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	conn, err := dialResponsesWebSocketWithTimeout("ws"+strings.TrimPrefix(server.URL, "http"), downstreamKey, timeout)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`)); err != nil {
		t.Fatal(err)
	}
	opcode, event, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeResponsesWebSocketEvent(opcode, event)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Status != 400 {
		t.Fatalf("terminal event = %s", event)
	}
	started := time.Now()
	err = waitForWebSocketPeerCloseWithTimeout(conn, timeout)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("error = %v, want timeout rather than peer close", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("terminal WebSocket timeout elapsed = %v", elapsed)
	}
}

func waitForWebSocketPeerClose(conn *websocket.Conn) error {
	return waitForWebSocketPeerCloseWithTimeout(conn, integrationIOTimeout)
}

func waitForWebSocketPeerCloseWithTimeout(conn *websocket.Conn, timeout time.Duration) error {
	if err := setDeadline(conn.UnderlyingConn(), timeout); err != nil {
		return err
	}
	_, _, err := conn.ReadMessage()
	if err == nil {
		return errors.New("connection stayed open after terminal 400 event")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("terminal 400 was not followed by peer closure: %w", err)
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) || errors.Is(err, io.EOF) {
		return nil
	}
	return fmt.Errorf("terminal 400 close read: %w", err)
}

func TestResponsesWebSocketSecondTurnBlocksLoadedToolDefinition(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [SECRET]\nscope:\n  roles: [tool]\n")
	conn := dialResponsesWebSocket(t, cpa.wsURL, downstreamKey)
	t.Cleanup(func() { _ = conn.Close() })

	first := []byte(`{"type":"response.create","model":"censorship-integration-model","input":"plain"}`)
	if err := conn.WriteMessage(websocket.TextMessage, first); err != nil {
		t.Fatal(err)
	}
	if messages := readUntilCompleted(t, conn); len(messages) == 0 {
		t.Fatal("first turn returned no completion messages")
	}
	if upstream.arrivalCount() != 1 {
		t.Fatalf("first-turn upstream arrivals = %d, want 1", upstream.arrivalCount())
	}

	second := []byte(`{"type":"response.create","model":"censorship-integration-model","input":[{"type":"tool_search_output","tools":[{"type":"function","name":"safe","description":"SECRET loaded"}]}]}`)
	if err := conn.WriteMessage(websocket.TextMessage, second); err != nil {
		t.Fatal(err)
	}
	opcode, event, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeResponsesWebSocketEvent(opcode, event)
	if err != nil || decoded.Status != 400 {
		t.Fatalf("event=%s error=%v", event, err)
	}
	if err := waitForWebSocketPeerClose(conn); err != nil {
		t.Fatalf("terminal 400 was not followed by peer closure: %v", err)
	}
	if upstream.arrivalCount() != 1 {
		t.Fatalf("second-turn upstream arrivals = %d, want 1", upstream.arrivalCount())
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
	decoded, err := decodeResponsesWebSocketEvent(opcode, event)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Status != 400 {
		t.Fatalf("opcode=%d event=%s", opcode, event)
	}
	if len(decoded.Error.Term) != 0 || len(decoded.Error.Role) != 0 {
		t.Fatalf("WebSocket unexpectedly retained plugin direct body: %s", event)
	}
	if err := waitForWebSocketPeerClose(conn); err != nil {
		t.Fatalf("terminal 400 was not followed by peer closure: %v", err)
	}
	if upstream.arrivalCount() != 0 {
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
		decoded, err := decodeResponsesWebSocketEvent(opcode, payload)
		if err != nil {
			return nil, err
		}
		messages = append(messages, wsMessage{Opcode: opcode, Payload: bytes.Clone(payload)})
		if decoded.Type == "response.completed" {
			return messages, nil
		}
	}
}

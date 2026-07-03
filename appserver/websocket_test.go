package appserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/gorilla/websocket"
)

func TestServeWebSocketRoundTripAndDaemonStop(t *testing.T) {
	server := NewServer(WithDaemonService(NewDaemonService(WithDaemonTransport("websocket"))))
	upgrader := websocket.Upgrader{}
	errCh := make(chan error, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		errCh <- ServeWebSocket(r.Context(), server, conn)
	}))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	defer conn.Close()

	writeWebSocketMessage(t, conn, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"ws-test"}}}`)
	initResp := readWebSocketResponse(t, conn)
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error)
	}

	writeWebSocketMessage(t, conn, `{"method":"initialized"}`)
	writeWebSocketMessage(t, conn, `{"id":"status","method":"daemon/status","params":{}}`)
	statusResp := readWebSocketResponse(t, conn)
	if statusResp.Error != nil {
		t.Fatalf("daemon/status error: %v", statusResp.Error)
	}
	var status DaemonStatus
	if err := json.Unmarshal(statusResp.Result, &status); err != nil {
		t.Fatalf("decode daemon/status: %v", err)
	}
	if status.Transport != "websocket" || status.Status != "running" {
		t.Fatalf("daemon/status = %#v", status)
	}

	writeWebSocketMessage(t, conn, `{"id":"stop","method":"daemon/stop","params":{"reason":"ws test"}}`)
	stopResp := readWebSocketResponse(t, conn)
	if stopResp.Error != nil {
		t.Fatalf("daemon/stop error: %v", stopResp.Error)
	}
	var stop DaemonStopResult
	if err := json.Unmarshal(stopResp.Result, &stop); err != nil {
		t.Fatalf("decode daemon/stop: %v", err)
	}
	if !stop.OK || !stop.Stopping {
		t.Fatalf("daemon/stop = %#v", stop)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeWebSocket: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeWebSocket did not return after daemon/stop")
	}
}

func writeWebSocketMessage(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		t.Fatalf("write websocket message: %v", err)
	}
}

func readWebSocketResponse(t *testing.T, conn *websocket.Conn) protocol.Response {
	t.Helper()
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket response: %v", err)
	}
	var resp protocol.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("decode websocket response %q: %v", data, err)
	}
	return resp
}

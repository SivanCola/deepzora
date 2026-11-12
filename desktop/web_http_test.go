package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
)

func localWebFixture(t *testing.T) (*localWeb, *httptest.Server) {
	t.Helper()
	t.Setenv("DEEPZORA_HOME", t.TempDir())
	service := NewService(ServiceOptions{Headless: true})
	if err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := service.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	server := httptest.NewUnstartedServer(nil)
	handler, err := newLocalWeb(service, fstest.MapFS{
		"index.html":    {Data: []byte("<html><head></head><body>Deepzora</body></html>")},
		"assets/app.js": {Data: []byte("console.log('UI')")},
	}, server.Listener.Addr().String(), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(func() { handler.close(); server.Close() })
	return handler, server
}

type webLogin struct {
	cookie *http.Cookie
	key    string
}

func webSocketHeader(server *httptest.Server, origin string, login webLogin) http.Header {
	header := http.Header{"Origin": {origin}, "Cookie": {login.cookie.String()}}
	if login.key != "" {
		header.Set("Sec-WebSocket-Protocol", webSocketProtocol+", "+webSocketKeyPrefix+login.key)
	}
	return header
}

func webCookie(t *testing.T, s *localWeb, server *httptest.Server) webLogin {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/web/auth", nil)
	req.Header.Set("Origin", server.URL)
	req.Header.Set("Authorization", "Bearer "+s.token)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("auth: %s", response.Status)
	}
	var body struct {
		SocketKey string `json:"socketKey"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.SocketKey == "" {
		t.Fatalf("auth body = %+v, %v", body, err)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v", cookies)
	}
	// The cookie reaches every loopback port, so it must not be the access
	// token or the socket key.
	if cookies[0].Value == s.token || cookies[0].Value == body.SocketKey || body.SocketKey == s.token {
		t.Fatal("cookie, socket key and access token must be distinct secrets")
	}
	return webLogin{cookie: cookies[0], key: body.SocketKey}
}

func webSocket(t *testing.T, server *httptest.Server, login webLogin) *websocket.Conn {
	t.Helper()
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/web/socket", webSocketHeader(server, server.URL, login))
	if err != nil {
		t.Fatalf("socket: %v (%v)", err, response)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ready struct {
		Ready  bool   `json:"ready"`
		Digest string `json:"digest"`
	}
	if err := conn.ReadJSON(&ready); err != nil || !ready.Ready || !strings.HasPrefix(ready.Digest, "sha256:") {
		t.Fatalf("ready = %+v, %v", ready, err)
	}
	return conn
}

func TestLocalWebAuthenticationAndOrigin(t *testing.T) {
	s, server := localWebFixture(t)
	for _, tc := range []struct {
		path, method, origin, host, token string
		want                              int
	}{
		{"/", "GET", "", "", "", 200},
		{"/assets/app.js", "GET", "", "", "", 200},
		{"/", "GET", "", "evil.test", "", 403},
		{"/", "GET", "https://evil.test", "", "", 403},
		{"/web/auth", "POST", server.URL, "", "wrong", 401},
		{"/web/auth", "GET", server.URL, "", s.token, 401},
		{"/web/auth", "POST", "", "", s.token, 401},
		{"/web/auth", "POST", "null", "", s.token, 403},
		{"/web/socket", "GET", server.URL, "", "", 401},
		{"/workspace-media/private", "GET", "", "", "", 401},
	} {
		req, _ := http.NewRequest(tc.method, server.URL+tc.path, nil)
		if tc.host != "" {
			req.Host = tc.host
		}
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != tc.want {
			t.Errorf("%+v: %s, %s", tc, res.Status, body)
		}
		if strings.Contains(string(body), s.token) {
			t.Fatal("access token leaked into response")
		}
	}
	login := webCookie(t, s, server)
	socketURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/web/socket"
	for _, origin := range []string{"", "https://evil.test", "null"} {
		_, res, err := websocket.DefaultDialer.Dial(socketURL, webSocketHeader(server, origin, login))
		if err == nil {
			t.Fatal("accepted unauthenticated WebSocket origin")
		}
		_ = res.Body.Close()
	}
	// A cookie picked up by another loopback port cannot open the command
	// socket, with or without a guessed key.
	for _, stolen := range []webLogin{{cookie: login.cookie}, {cookie: login.cookie, key: s.token}, {cookie: login.cookie, key: login.cookie.Value}} {
		_, res, err := websocket.DefaultDialer.Dial(socketURL, webSocketHeader(server, server.URL, stolen))
		if err == nil {
			t.Fatal("cookie alone opened the command socket")
		}
		_ = res.Body.Close()
	}
}

func TestLocalWebCommandBurstKeepsConnection(t *testing.T) {
	s, server := localWebFixture(t)
	conn := webSocket(t, server, webCookie(t, s, server))
	// An unaddressable request is ignored, and more commands than the
	// concurrency limit are all answered on the same connection.
	if err := conn.WriteJSON(map[string]any{"id": 0, "method": "Version"}); err != nil {
		t.Fatal(err)
	}
	const burst = 200
	for id := 1; id <= burst; id++ {
		if err := conn.WriteJSON(map[string]any{"id": id, "method": "Version", "args": []any{}}); err != nil {
			t.Fatal(err)
		}
	}
	answered := map[int]bool{}
	for len(answered) < burst {
		var reply struct {
			ID    int    `json:"id"`
			Value string `json:"value"`
		}
		if err := conn.ReadJSON(&reply); err != nil {
			t.Fatalf("connection dropped after %d of %d replies: %v", len(answered), burst, err)
		}
		if reply.Value != version {
			t.Fatalf("reply = %+v", reply)
		}
		answered[reply.ID] = true
	}
}

func TestLocalWebRealBindingsEventsAndReload(t *testing.T) {
	s, server := localWebFixture(t)
	login := webCookie(t, s, server)
	conn := webSocket(t, server, login)
	if err := conn.WriteJSON(map[string]any{"id": 1, "method": "Version", "args": []any{}}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID    int    `json:"id"`
		Value string `json:"value"`
	}
	if err := conn.ReadJSON(&result); err != nil || result.ID != 1 || result.Value != version {
		t.Fatalf("real Version = %+v, %v", result, err)
	}
	if err := conn.WriteJSON(map[string]any{"id": 2, "method": "UnknownCommand", "args": []any{}}); err != nil {
		t.Fatal(err)
	}
	var failed struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := conn.ReadJSON(&failed); err != nil || failed.Error.Code != -32601 {
		t.Fatalf("unknown command = %+v, %v", failed, err)
	}
	s.emit(context.Background(), "agent:event", map[string]string{"kind": "test"})
	var event struct {
		Event string            `json:"event"`
		Args  []json.RawMessage `json:"args"`
	}
	if err := conn.ReadJSON(&event); err != nil || event.Event != "agent:event" || len(event.Args) != 1 {
		t.Fatalf("event = %+v, %v", event, err)
	}
	// A refreshed page receives the same service, with the previous controller
	// page explicitly disconnected instead of racing two active UIs.
	next := webSocket(t, server, login)
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("old controlling page remained connected")
	}
	if err := next.WriteJSON(map[string]any{"id": 3, "method": "Version", "args": []any{}}); err != nil {
		t.Fatal(err)
	}
	if err := next.ReadJSON(&result); err != nil || result.ID != 3 {
		t.Fatalf("reloaded Version = %+v, %v", result, err)
	}
}

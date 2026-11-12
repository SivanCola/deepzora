package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"deepzora/desktop/internal/hostrpc"
	"deepzora/internal/extension/rpcwire"
	"deepzora/internal/secrets"

	"github.com/gorilla/websocket"
)

type localWeb struct {
	app         *App
	service     *Service
	contract    hostrpc.Contract
	assets      http.Handler
	files       fs.FS
	index       []byte
	host, token string
	// Browsers send a loopback cookie to every port on the host, so the cookie
	// alone only reaches read-only media routes. The command socket also needs
	// the key, which the page keeps in origin-scoped storage.
	session, socketKey string
	mu                 sync.Mutex
	client             *webClient
}

const (
	webSocketProtocol  = "deepzora-web"
	webSocketKeyPrefix = "deepzora-web-key."
	// Bursts of stream events must not disconnect a page that is briefly slow.
	webClientQueueSize = 4096
)

type webClient struct {
	conn  *websocket.Conn
	queue chan any
	done  chan struct{}
	once  sync.Once
}

func (c *webClient) close() { c.once.Do(func() { close(c.done); _ = c.conn.Close() }) }

// closeBehind tells the page why it was dropped, so it can ask for a reload
// that rebuilds the view from the service instead of reporting a lost server.
func (c *webClient) closeBehind() {
	message := websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "event backlog")
	_ = c.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))
	c.close()
}

func (c *webClient) send(value any) {
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case c.queue <- value:
	case <-c.done:
	default:
		c.closeBehind()
	}
}

func newLocalWeb(service *Service, assets fs.FS, host, token string) (*localWeb, error) {
	contract, err := service.Contract()
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return nil, err
	}
	index = bytes.Replace(index, []byte("<head>"), []byte("<head><meta name=\"deepzora-web\" content=\"1\">"), 1)
	session, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	socketKey, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	return &localWeb{app: service.app, service: service, contract: contract, assets: http.FileServerFS(assets), files: assets, index: index, host: host, token: token, session: session, socketKey: socketKey}, nil
}

func (s *localWeb) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Exact Host validation prevents DNS rebinding. Only this service's loopback
	// origin may use credentials, including WebSocket upgrades.
	if r.Host != s.host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+s.host) {
		http.Error(w, "local origin required", http.StatusForbidden)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/web/auth" {
		if r.Method != http.MethodPost || r.Header.Get("Origin") != "http://"+s.host || !secretEqual(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), s.token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "deepzora_web", Value: s.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"socketKey": s.socketKey})
		return
	}
	if r.URL.Path == "/web/socket" {
		if !s.validSession(r) || !s.validSocketKey(r) || r.Header.Get("Origin") != "http://"+s.host {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.socket(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/web/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodHead {
			_, _ = w.Write(s.index)
		}
		return
	}
	// UI assets are public. Local files, attachments and theme routes require
	// the same cookie and exact origin as commands.
	assetPath := strings.TrimPrefix(r.URL.Path, "/")
	if info, err := fs.Stat(s.files, assetPath); err == nil && !info.IsDir() && !strings.HasPrefix(assetPath, ".") {
		s.assets.ServeHTTP(w, r)
		return
	}
	{
		if !s.validSession(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler := s.app.themeAssetMiddleware()(s.assets)
		handler = s.app.workspaceMediaMiddleware()(handler)
		handler = s.app.remoteMarkdownImageMiddleware()(handler)
		handler.ServeHTTP(w, r)
		return
	}
}

func secretEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *localWeb) validSession(r *http.Request) bool {
	cookie, err := r.Cookie("deepzora_web")
	return err == nil && secretEqual(cookie.Value, s.session)
}

// The key travels as an offered subprotocol: browsers cannot set headers on a
// WebSocket, and a handshake field stays out of recorded message frames.
func (s *localWeb) validSocketKey(r *http.Request) bool {
	for _, protocol := range websocket.Subprotocols(r) {
		if key, ok := strings.CutPrefix(protocol, webSocketKeyPrefix); ok && secretEqual(key, s.socketKey) {
			return true
		}
	}
	return false
}
func (s *localWeb) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.close()
	}
}
func (s *localWeb) emit(_ context.Context, name string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.send(map[string]any{"event": name, "args": secrets.Project(args)})
	}
}

func (s *localWeb) socket(w http.ResponseWriter, r *http.Request) {
	upgrade := websocket.Upgrader{Subprotocols: []string{webSocketProtocol}, CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "http://"+s.host }}
	conn, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &webClient{conn: conn, queue: make(chan any, webClientQueueSize), done: make(chan struct{})}
	defer client.close()
	s.mu.Lock()
	if s.client != nil {
		s.client.close()
	}
	s.client = client
	contract := s.contract
	client.send(map[string]any{"ready": true, "protocolVersion": contract.ProtocolVersion, "digest": contract.Digest()})
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.client == client {
			s.client = nil
		}
		s.mu.Unlock()
	}()
	go func() {
		defer client.close()
		for {
			select {
			case value := <-client.queue:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if conn.WriteJSON(value) != nil {
					return
				}
			case <-client.done:
				return
			}
		}
	}()
	conn.SetReadLimit(8 << 20)
	admission := make(chan struct{}, 64)
	for {
		var request struct {
			ID     uint64            `json:"id"`
			Method string            `json:"method"`
			Args   []json.RawMessage `json:"args"`
		}
		if err := conn.ReadJSON(&request); err != nil {
			return
		}
		if request.ID == 0 {
			// Unaddressable, so it cannot be answered; it must not cost the
			// page its connection and every command still in flight.
			continue
		}
		// Reading pauses at the concurrency limit, so a burst of commands
		// waits its turn instead of dropping the connection.
		select {
		case admission <- struct{}{}:
		case <-client.done:
			return
		}
		go func() {
			defer func() { <-admission }()
			// The service owns execution. Closing/reloading the page never cancels
			// an accepted submission or releases a session's writer lease.
			value, err := s.service.Invoke(s.app.bootContext(), request.Method, request.Args)
			reply := map[string]any{"id": request.ID, "value": value}
			if err != nil {
				rpcErr := &rpcwire.RPCError{Code: -32000, Message: fmt.Sprint(err)}
				_ = errors.As(err, &rpcErr)
				reply["error"] = rpcErr
			}
			client.send(reply)
		}()
	}
}

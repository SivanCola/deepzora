package serve

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"deepzora/internal/config"
)

// authMode represents the authentication mode for the serve frontend.
type authMode int

const (
	authInvalid authMode = iota // invalid config; deny all requests
	authNone                    // explicit opt-out; mutations still need the launch token
	authToken                   // pre-shared token in URL or cookie
)

const (
	cookieToken     = "deepzora_token"    // holds the token for token mode
	tokenByteLen    = 32                  // 256-bit random token
	sessionDuration = 30 * 24 * time.Hour // how long the token cookie lasts
)

// NormalizeAuthMode normalizes and validates the serve auth mode.
func NormalizeAuthMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "none"
	}
	switch mode {
	case "none", "token":
		return mode, nil
	default:
		return "", fmt.Errorf("auth mode must be none or token, got %q", mode)
	}
}

// authGate is the authentication middleware and its runtime state.
type authGate struct {
	mode        authMode
	token       string // pre-shared token (token mode)
	behindProxy bool   // trust X-Forwarded-Proto headers
	// capabilities reports what this serve advertises on the token handshake
	// (e.g. the browser broker); nil means no capability header.
	capabilities func() []string
}

// newAuthGate creates the auth middleware from the serve config. For token mode
// without a configured token, it generates a random one.
func newAuthGate(cfg config.ServeConfig) *authGate {
	ag := &authGate{
		behindProxy: cfg.BehindProxy,
	}
	mode, err := NormalizeAuthMode(cfg.AuthMode)
	if err != nil {
		ag.mode = authInvalid
		return ag
	}
	switch mode {
	case "token":
		ag.mode = authToken
		ag.token = strings.TrimSpace(cfg.Token)
		if ag.token == "" {
			ag.token = generateToken()
		}
	default:
		ag.mode = authNone
		ag.token = strings.TrimSpace(cfg.Token)
		if ag.token == "" {
			ag.token = generateToken()
		}
	}
	return ag
}

// Token returns the launch token: the shared token in token mode, or the token
// mutations need when authentication is off.
func (ag *authGate) Token() string { return ag.token }

// Mode returns the auth mode name as a string.
func (ag *authGate) Mode() string {
	switch ag.mode {
	case authToken:
		return "token"
	case authInvalid:
		return "invalid"
	default:
		return "none"
	}
}

// middleware returns an http.Handler that wraps next with authentication checks.
func (ag *authGate) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ag.mode == authInvalid {
			ag.deny(w, r)
			return
		}
		if ag.mode == authNone {
			if r.URL.Path == "/auth/token" {
				ag.handleTokenBootstrap(w, r)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/auth/token" {
			ag.handleTokenBootstrap(w, r)
			return
		}
		// Let the inert shell trade its URL fragment for an HttpOnly cookie
		// before API or SSE calls.
		if r.URL.Query().Get("token") == "" && tokenBootstrapPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		ag.checkToken(w, r, next)
	})
}

func tokenBootstrapPublicPath(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if r.URL.Path == "/" || r.URL.Path == "/assets/logo-wordmark.svg" {
		return true
	}
	// Only one non-empty session segment is an inert shell entry point; this
	// prevents API-like paths from becoming public in token mode.
	const prefix = "/sessions/"
	id := strings.TrimPrefix(r.URL.Path, prefix)
	return id != r.URL.Path && id != "" && !strings.Contains(id, "/")
}

// handleTokenBootstrap validates a token delivered from the URL fragment by
// the Web shell. The token travels in a bounded JSON body rather than the URL,
// keeping it out of request lines, access logs, browser history, and referrers.
func (ag *authGate) handleTokenBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	if !strings.EqualFold(strings.TrimSpace(contentType), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var body struct {
		Token string `json:"token"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Token), []byte(ag.token)) != 1 {
		ag.deny(w, r)
		return
	}
	ag.setAuthCookie(w, r, &http.Cookie{
		Name:     cookieToken,
		Value:    ag.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
	if ag.capabilities != nil {
		if caps := ag.capabilities(); len(caps) > 0 {
			w.Header().Set(capabilitiesHeader, strings.Join(caps, ","))
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// checkToken validates the cookie or Authorization bearer established by bootstrap.
func (ag *authGate) checkToken(w http.ResponseWriter, r *http.Request, next http.Handler) {
	// 1. Cookie or Authorization bearer (fast path).
	if ag.presentsLaunchToken(r) {
		next.ServeHTTP(w, r)
		return
	}

	ag.deny(w, r)
}

// deny sends a 401 response. The message is intentionally generic to avoid
// leaking information about which auth mode is active.
func (ag *authGate) deny(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("Unauthorized\n"))
}

func (ag *authGate) setAuthCookie(w http.ResponseWriter, r *http.Request, c *http.Cookie) {
	c.Secure = ag.authCookieSecure(r)
	// codeql[go/cookie-secure-not-set] Secure cookies are only sent back over HTTPS; plain-HTTP serve must keep token auth usable.
	http.SetCookie(w, c)
}

func (ag *authGate) authCookieSecure(r *http.Request) bool {
	return ag.isTLS(r)
}

// generateToken returns a cryptographically random URL-safe token.
func generateToken() string {
	b := make([]byte, tokenByteLen)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failure is fatal for token generation.
		panic("serve/auth: crypto/rand.Read failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// isTLS reports whether the request arrived over TLS. It trusts
// X-Forwarded-Proto only when behindProxy is true.
func (ag *authGate) isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if ag.behindProxy {
		return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
	return false
}

func isLoopbackHost(hostport string) bool {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return false
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// PlainHTTPAuthWarning returns a warning string when serve is exposed on a
// non-loopback plain-HTTP listener. The listener may still be valid for a
// trusted LAN or reverse-proxy setup, but users should see the risk explicitly
// — loudest for the unauthenticated case, which used to be the silent one.
func PlainHTTPAuthWarning(cfg config.ServeConfig, addr string) string {
	mode, err := NormalizeAuthMode(cfg.AuthMode)
	if err != nil || isLoopbackHost(addr) {
		return ""
	}
	if mode == "none" {
		return "warning: serve is listening on non-loopback HTTP with authentication disabled; anyone on this network can read sessions and history, and the launch token that changes need crosses it in clear — bind to 127.0.0.1 or set serve.auth_mode"
	}
	return "warning: authenticated serve is listening on non-loopback HTTP; use HTTPS via a trusted reverse proxy or bind to 127.0.0.1 for local-only access"
}

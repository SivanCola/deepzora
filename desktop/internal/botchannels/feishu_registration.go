package botchannels

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

type FeishuRegistration struct {
	Code      string `json:"-"`
	Image     string
	Domain    string
	Status    string
	AppID     string
	Secret    string `json:"-"`
	UserID    string
	Interval  int
	ExpiresIn int
}

func registrationRequest(ctx context.Context, client *http.Client, domain string, form url.Values, out any) error {
	origin := "https://accounts.feishu.cn"
	if domain == "lark" {
		origin = "https://accounts.larksuite.com"
	} else if domain != "feishu" {
		return errors.New("unsupported registration domain")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/oauth/v1/app/registration", strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("invalid registration request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(req)
	if err != nil {
		return errors.New("app registration connection failed")
	}
	defer response.Body.Close()
	// OAuth pending/denied may be represented by an HTTP 400 JSON envelope.
	if (response.StatusCode < 200 || response.StatusCode >= 300) && response.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("app registration HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, out) != nil {
		return errors.New("invalid app registration response")
	}
	return nil
}
func BeginFeishuRegistration(ctx context.Context, client *http.Client) (FeishuRegistration, error) {
	var reply struct {
		Code     string `json:"device_code"`
		URL      string `json:"verification_uri_complete"`
		Interval int    `json:"interval"`
		Expires  int    `json:"expire_in"`
	}
	// 官方 SDK 和 ZCode 都从飞书 issuer 开始；Lark 租户在轮询结果中切换。
	err := registrationRequest(ctx, client, "feishu", url.Values{"action": {"begin"}, "archetype": {"PersonalAgent"}, "auth_method": {"client_secret"}, "request_user_info": {"open_id"}}, &reply)
	if err != nil {
		return FeishuRegistration{}, err
	}
	if reply.Code == "" || reply.URL == "" {
		return FeishuRegistration{}, errors.New("app registration did not return a QR code")
	}
	uri, err := url.Parse(reply.URL)
	if err != nil || uri.Scheme != "https" || uri.User != nil {
		return FeishuRegistration{}, errors.New("invalid app registration QR URL")
	}
	host := strings.ToLower(uri.Hostname())
	if host != "accounts.feishu.cn" && host != "accounts.larksuite.com" && host != "open.feishu.cn" && host != "open.larksuite.com" {
		return FeishuRegistration{}, errors.New("unrecognized app registration QR host")
	}
	query := uri.Query()
	query.Set("from", "sdk")
	query.Set("source", "go-sdk/deepzora")
	query.Set("tp", "sdk")
	uri.RawQuery = query.Encode()
	png, err := qrcode.Encode(uri.String(), qrcode.Medium, 256)
	if err != nil {
		return FeishuRegistration{}, errors.New("cannot render app registration QR")
	}
	expires := reply.Expires
	if expires <= 0 {
		expires = 600
	}
	interval := reply.Interval
	if interval <= 0 {
		interval = 5
	}
	return FeishuRegistration{Code: reply.Code, Image: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), Domain: "feishu", Status: "pending", Interval: min(interval, 30), ExpiresIn: min(expires, 900)}, nil
}
func PollFeishuRegistration(ctx context.Context, client *http.Client, domain, code string) (FeishuRegistration, error) {
	var reply struct {
		AppID    string `json:"client_id"`
		Secret   string `json:"client_secret"`
		Error    string `json:"error"`
		UserInfo struct {
			OpenID string `json:"open_id"`
			Brand  string `json:"tenant_brand"`
		} `json:"user_info"`
	}
	err := registrationRequest(ctx, client, domain, url.Values{"action": {"poll"}, "device_code": {code}}, &reply)
	if err != nil {
		return FeishuRegistration{}, err
	}
	result := FeishuRegistration{Domain: domain, Status: "pending", Interval: 5}
	if reply.UserInfo.Brand == "lark" && domain != "lark" {
		result.Domain = "lark"
		result.Interval = 1
		return result, nil
	}
	if reply.AppID != "" && reply.Secret != "" {
		userID := strings.TrimSpace(reply.UserInfo.OpenID)
		if len(userID) > 256 || strings.ContainsAny(userID, "\r\n\x00") {
			return FeishuRegistration{}, errors.New("invalid registration user identity")
		}
		result.Status = "success"
		result.AppID = reply.AppID
		result.Secret = reply.Secret
		result.UserID = userID
		return result, nil
	}
	switch reply.Error {
	case "", "authorization_pending":
	case "slow_down":
		result.Interval = 10
	case "access_denied":
		result.Status = "denied"
	case "expired_token":
		result.Status = "expired"
	default:
		return FeishuRegistration{}, errors.New("app registration failed; retry or use an existing app")
	}
	return result, nil
}

func PrivateFeishuRecipient(config Config) (string, string) {
	if recipient, ok := strings.CutPrefix(config.ChatID, "openid:"); ok {
		return "open_id", recipient
	}
	return "chat_id", config.ChatID
}

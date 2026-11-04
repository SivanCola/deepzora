package botchannels

import (
	"bytes"
	"context"
	"deepzora/internal/netclient"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var providerHTTP = &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type temporaryError struct{ message string }

func (e *temporaryError) Error() string { return e.message }
func Retryable(err error) bool          { var transient *temporaryError; return errors.As(err, &transient) }

func request(ctx context.Context, client *http.Client, url string, headers map[string]string, body any, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	method := "POST"
	if body == nil {
		method = "GET"
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid provider request")
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return &temporaryError{message: "provider connection failed"}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if res.StatusCode >= 500 || res.StatusCode == 429 {
			return &temporaryError{message: fmt.Sprintf("provider HTTP %d", res.StatusCode)}
		}
		return fmt.Errorf("provider HTTP %d", res.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil {
		return &temporaryError{message: "provider response interrupted"}
	}
	if len(data) > 2<<20 {
		return fmt.Errorf("provider response exceeded limit")
	}
	if json.Unmarshal(data, out) != nil {
		return fmt.Errorf("invalid provider response")
	}
	return nil
}

type Incoming struct{ ID, ChatID, SenderID, Text, ContextToken string }
type Adapter interface {
	Run(context.Context, func(Incoming) error, func(string)) error
	Send(context.Context, string) error
}

func HTTPClient(spec netclient.ProxySpec) (*http.Client, error) {
	client, err := netclient.NewHTTPClient(spec, netclient.TransportOptions{})
	if err != nil {
		return nil, fmt.Errorf("invalid bot proxy configuration")
	}
	client.Timeout = 45 * time.Second
	client.CheckRedirect = providerHTTP.CheckRedirect
	return client, nil
}
func NewAdapter(c Saved, client *http.Client) Adapter {
	switch c.Provider {
	case "telegram":
		return &telegramAdapter{config: c, client: client, base: "https://api.telegram.org"}
	case "weixin":
		return &weixinAdapter{config: c, client: client, base: "https://ilinkai.weixin.qq.com"}
	default:
		return &feishuAdapter{config: c, client: client}
	}
}

func chunks(text string, limit int) []string {
	r := []rune(text)
	var result []string
	for len(r) > 0 {
		n := min(limit, len(r))
		result = append(result, string(r[:n]))
		r = r[n:]
	}
	return result
}

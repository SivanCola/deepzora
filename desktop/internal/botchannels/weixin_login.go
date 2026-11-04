package botchannels

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

func BeginWeixinLogin(ctx context.Context, client *http.Client) (code, dataURL string, err error) {
	var reply struct {
		Ret  int    `json:"ret"`
		Code string `json:"qrcode"`
		URL  string `json:"qrcode_img_content"`
	}
	err = request(ctx, client, "https://ilinkai.weixin.qq.com/ilink/bot/get_bot_qrcode?bot_type=3", map[string]string{"iLink-App-ClientVersion": "1"}, nil, &reply)
	if err != nil {
		return "", "", err
	}
	if reply.Ret != 0 || reply.Code == "" || reply.URL == "" {
		return "", "", fmt.Errorf("Weixin did not return a login QR code")
	}
	png, err := qrcode.Encode(reply.URL, qrcode.Medium, 256)
	if err != nil {
		return "", "", fmt.Errorf("cannot create login QR code")
	}
	return reply.Code, "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

type WeixinLoginResult struct {
	Status string
	Token  string `json:"-"`
	UserID string
}

func PollWeixinLogin(ctx context.Context, client *http.Client, code string) (WeixinLoginResult, error) {
	var reply struct {
		Status string `json:"status"`
		Token  string `json:"bot_token"`
		UserID string `json:"ilink_user_id"`
		Ret    int    `json:"ret"`
	}
	err := request(ctx, client, "https://ilinkai.weixin.qq.com/ilink/bot/get_qrcode_status?qrcode="+url.QueryEscape(code), map[string]string{"iLink-App-ClientVersion": "1"}, nil, &reply)
	if err != nil {
		return WeixinLoginResult{}, err
	}
	if reply.Ret != 0 {
		return WeixinLoginResult{}, fmt.Errorf("Weixin login rejected")
	}
	switch strings.ToLower(reply.Status) {
	case "confirmed", "success", "authorized":
		if reply.Token == "" {
			return WeixinLoginResult{}, fmt.Errorf("Weixin did not return a bot credential")
		}
		// 扫码用户的 ilink_user_id 才是私聊访问对象；不能误用机器人的
		// ilink_bot_id。旧实现只保留 Token，导致界面要求用户手工查两个 ID。
		userID := strings.TrimSpace(reply.UserID)
		if len(userID) > 256 || strings.ContainsAny(userID, "\r\n\x00") {
			return WeixinLoginResult{}, fmt.Errorf("Weixin returned an invalid user identity")
		}
		return WeixinLoginResult{Status: "success", Token: reply.Token, UserID: userID}, nil
	case "scaned", "scanned":
		return WeixinLoginResult{Status: "scanned"}, nil
	case "expired", "timeout", "cancelled":
		return WeixinLoginResult{Status: "expired"}, nil
	case "wait", "pending", "":
		return WeixinLoginResult{Status: "pending"}, nil
	default:
		return WeixinLoginResult{}, fmt.Errorf("unrecognized Weixin login state")
	}
}

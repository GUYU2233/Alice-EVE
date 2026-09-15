package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type DeviceAuthorization struct {
	AccountID  string `json:"accountId"`
	DeviceName string `json:"deviceName"`
	DeviceType string `json:"deviceType"`
}

type deviceAuthStart struct {
	Challenge string `json:"challenge"`
}
type deviceAuthResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	DeviceToken  string `json:"deviceToken"`
	DeviceID     string `json:"deviceId"`
	AccountID    string `json:"accountId"`
}

func (r *RelayClient) AuthorizeDevice(ctx context.Context, auth DeviceAuthorization) (deviceAuthResponse, error) {
	auth.AccountID = strings.TrimSpace(auth.AccountID)
	auth.DeviceName = strings.TrimSpace(auth.DeviceName)
	if auth.AccountID == "" || auth.DeviceName == "" {
		return deviceAuthResponse{}, fmt.Errorf("accountId and deviceName are required")
	}
	body, err := json.Marshal(map[string]string{"accountId": auth.AccountID, "deviceName": auth.DeviceName, "deviceType": "desktop"})
	if err != nil {
		return deviceAuthResponse{}, err
	}
	var start deviceAuthStart
	if err = r.do(ctx, http.MethodPost, "/api/v1/auth/device/start", body, &start); err != nil {
		return deviceAuthResponse{}, err
	}
	if start.Challenge == "" {
		return deviceAuthResponse{}, fmt.Errorf("authorization challenge missing")
	}
	b, _ := json.Marshal(map[string]string{"challenge": start.Challenge})
	var out deviceAuthResponse
	if err = r.do(ctx, http.MethodPost, "/api/v1/auth/device/complete", b, &out); err != nil {
		return deviceAuthResponse{}, err
	}
	if out.DeviceToken == "" || out.DeviceID == "" {
		return deviceAuthResponse{}, fmt.Errorf("device credentials missing")
	}
	if err = r.SetTokenSecure(out.DeviceToken); err != nil {
		return deviceAuthResponse{}, err
	}
	if out.AccessToken != "" && out.RefreshToken != "" {
		_ = r.SaveOAuthCredentials(OAuthCredentials{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, DeviceToken: out.DeviceToken, DeviceID: out.DeviceID, AccountID: out.AccountID})
	}
	return out, nil
}

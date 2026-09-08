package pan

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

// Public application identity used by JavdBviewed's OpenList scan login.
const openListAppID = "100197303"

type LoginState string

const (
	LoginWaiting    LoginState = "waiting"
	LoginScanned    LoginState = "scanned"
	LoginAuthorized LoginState = "authorized"
	LoginExpired    LoginState = "expired"
	LoginCanceled   LoginState = "canceled"
)

// Login keeps the provider's device code and PKCE verifier inside this package.
type Login struct {
	QRCode   []byte
	uid      string
	time     int64
	sign     string
	verifier string
}

type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (client *Client) BeginLogin(ctx context.Context) (*Login, error) {
	random := make([]byte, 48)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("generate 115 PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(random)
	digest := sha256.Sum256([]byte(verifier))
	// 115 specifies base64(SHA256(verifier)) and the method name "sha256".
	challenge := base64.StdEncoding.EncodeToString(digest[:])
	data, err := authRequest[struct {
		UID  string `json:"uid"`
		Time int64  `json:"time"`
		Sign string `json:"sign"`
	}](client, client.http.R().SetContext(ctx).SetFormData(map[string]string{
		"client_id":             openListAppID,
		"code_challenge":        challenge,
		"code_challenge_method": "sha256",
	}), http.MethodPost, passportURL+"/open/authDeviceCode")
	if err != nil {
		return nil, err
	}
	if data.UID == "" || data.Time == 0 || data.Sign == "" {
		return nil, fmt.Errorf("115 authorization response is missing device code fields")
	}
	response, err := client.request(
		client.http.R().SetContext(ctx).SetQueryParam("uid", data.UID),
		http.MethodGet, qrcodeURL+"/api/1.0/web/1.0/qrcode",
	)
	if err != nil {
		return nil, err
	}
	if http.DetectContentType(response.Body()) != "image/png" {
		return nil, fmt.Errorf("115 did not return a PNG login QR code")
	}
	return &Login{
		QRCode: response.Body(), uid: data.UID, time: data.Time,
		sign: data.Sign, verifier: verifier,
	}, nil
}

func (client *Client) LoginStatus(ctx context.Context, login *Login) (LoginState, error) {
	data, err := authRequest[struct {
		Status *int `json:"status"`
	}](client, client.http.R().SetContext(ctx).SetQueryParams(map[string]string{
		"uid":  login.uid,
		"time": strconv.FormatInt(login.time, 10),
		"sign": login.sign,
	}), http.MethodGet, qrcodeURL+"/get/status/")
	if err != nil {
		return "", err
	}
	if data.Status == nil {
		return "", fmt.Errorf("115 login response is missing status")
	}
	switch *data.Status {
	case 0:
		return LoginWaiting, nil
	case 1:
		return LoginScanned, nil
	case 2:
		return LoginAuthorized, nil
	case -1:
		return LoginExpired, nil
	case -2:
		return LoginCanceled, nil
	default:
		return "", fmt.Errorf("115 returned unknown login status %d", *data.Status)
	}
}

func (client *Client) ExchangeToken(ctx context.Context, login *Login) (Tokens, error) {
	return client.requestTokens(client.http.R().SetContext(ctx).SetFormData(map[string]string{
		"uid":           login.uid,
		"code_verifier": login.verifier,
	}), "/open/deviceCodeToToken")
}

func (client *Client) RefreshToken(ctx context.Context, refreshToken string) (Tokens, error) {
	return client.requestTokens(client.http.R().SetContext(ctx).SetFormData(map[string]string{
		"refresh_token": refreshToken,
	}), "/open/refreshToken")
}

func (client *Client) requestTokens(request *resty.Request, path string) (Tokens, error) {
	issuedAt := time.Now()
	data, err := authRequest[struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}](client, request, http.MethodPost, passportURL+path)
	if err != nil {
		return Tokens{}, err
	}
	if data.AccessToken == "" || data.RefreshToken == "" || data.ExpiresIn <= 0 {
		return Tokens{}, fmt.Errorf("115 token response is missing credentials or expiry")
	}
	return Tokens{
		AccessToken: data.AccessToken, RefreshToken: data.RefreshToken,
		ExpiresAt: issuedAt.Add(time.Duration(data.ExpiresIn) * time.Second),
	}, nil
}

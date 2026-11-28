package drive

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/guangya"
	"github.com/ppxb/miyabi/internal/pan"
)

const guangyaDeviceCodeResponse = `{"device_code":"device","user_code":"user","expires_in":120,"interval":2,` +
	`"verification_uri_complete":"https://account.guangyapan.com/__/auth/device/?user_code=user"}`

func newGuangyaLoginDrive(t *testing.T, handler http.HandlerFunc) *Drive {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := guangya.NewWithBases(server.URL, server.URL)
	d := &Drive{guangya: client}
	d.guangyaLogin.drive = d
	return d
}

func beginGuangyaLogin(t *testing.T, d *Drive) GuangyaLoginSession {
	t.Helper()
	session, err := d.BeginGuangyaLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || session.QRCodeURL == "" {
		t.Fatalf("session = %+v", session)
	}
	return session
}

func TestGuangyaLoginBeginsDeviceAuthorization(t *testing.T) {
	d := newGuangyaLoginDrive(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/device/code" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, guangyaDeviceCodeResponse)
	})
	session := beginGuangyaLogin(t, d)
	want := "https://account.guangyapan.com/__/auth/device/?user_code=user"
	if session.QRCodeURL != want {
		t.Fatalf("qr url = %q, want %q", session.QRCodeURL, want)
	}
}

func TestGuangyaLoginStatusWaitsForAuthorization(t *testing.T) {
	d := newGuangyaLoginDrive(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/device/code":
			fmt.Fprint(w, guangyaDeviceCodeResponse)
		case "/v1/auth/token":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"authorization_pending"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	session := beginGuangyaLogin(t, d)
	status, err := d.GuangyaLoginStatus(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != pan.LoginWaiting {
		t.Fatalf("state = %q", status.State)
	}
}

func TestGuangyaLoginExpiresStaleSession(t *testing.T) {
	d := newGuangyaLoginDrive(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, guangyaDeviceCodeResponse)
	})
	session := beginGuangyaLogin(t, d)
	d.mu.Lock()
	d.guangyaLogin.session.expiresAt = time.Now().Add(-time.Second)
	d.mu.Unlock()
	status, err := d.GuangyaLoginStatus(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != pan.LoginExpired {
		t.Fatalf("state = %q", status.State)
	}
}

func TestGuangyaLoginReplacedSessionIsCanceled(t *testing.T) {
	d := newGuangyaLoginDrive(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, guangyaDeviceCodeResponse)
	})
	first := beginGuangyaLogin(t, d)
	beginGuangyaLogin(t, d)
	status, err := d.GuangyaLoginStatus(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != pan.LoginCanceled {
		t.Fatalf("state = %q", status.State)
	}
}

func TestGuangyaLoginStoresAuthorizedAccount(t *testing.T) {
	d := newTestDrive(t, &stubClient{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/device/code":
			fmt.Fprint(w, guangyaDeviceCodeResponse)
		case "/v1/auth/token":
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"refresh","expires_in":7200}`)
		case "/v1/user/me":
			fmt.Fprint(w, `{"sub":"guangya-user","name":"Tester"}`)
		case "/nd.bizassets.s/v1/get_assets":
			fmt.Fprint(w, `{"msg":"success","data":{"totalSpaceSize":1024,"usedSpaceSize":0}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	d.guangya = guangya.NewWithBases(server.URL, server.URL)

	session := beginGuangyaLogin(t, d)
	status, err := d.GuangyaLoginStatus(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != pan.LoginAuthorized || status.AccountID != "guangya-user" {
		t.Fatalf("status = %+v", status)
	}
	tokens := testToken(t, d, "guangya-user")
	if tokens.AccessToken != "access" || tokens.RefreshToken != "refresh" {
		t.Fatalf("tokens = %+v", tokens)
	}

	// Repeated polls after success must keep reporting the same account.
	again, err := d.GuangyaLoginStatus(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.State != pan.LoginAuthorized || again.AccountID != "guangya-user" {
		t.Fatalf("repeated status = %+v", again)
	}
}

func TestGuangyaLoginReusesIssuedTokenAfterAccountFailure(t *testing.T) {
	d := newTestDrive(t, &stubClient{})
	tokenCalls, meCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/device/code":
			fmt.Fprint(w, guangyaDeviceCodeResponse)
		case "/v1/auth/token":
			tokenCalls++
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"refresh","expires_in":7200}`)
		case "/v1/user/me":
			meCalls++
			if meCalls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `{"sub":"guangya-user","name":"Tester"}`)
		case "/nd.bizassets.s/v1/get_assets":
			fmt.Fprint(w, `{"msg":"success","data":{"totalSpaceSize":1024,"usedSpaceSize":0}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	d.guangya = guangya.NewWithBases(server.URL, server.URL)

	session := beginGuangyaLogin(t, d)
	if _, err := d.GuangyaLoginStatus(t.Context(), session.ID); err == nil {
		t.Fatal("expected account lookup failure")
	}
	status, err := d.GuangyaLoginStatus(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != pan.LoginAuthorized || status.AccountID != "guangya-user" {
		t.Fatalf("status = %+v", status)
	}
	if tokenCalls != 1 {
		t.Fatalf("token calls = %d, want 1", tokenCalls)
	}
}

func TestGuangyaLoginUnknownSessionExpires(t *testing.T) {
	d := newGuangyaLoginDrive(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, guangyaDeviceCodeResponse)
	})
	status, err := d.GuangyaLoginStatus(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != pan.LoginExpired {
		t.Fatalf("state = %q", status.State)
	}
}

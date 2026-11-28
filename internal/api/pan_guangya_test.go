package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type guangyaLoginStub struct {
	DriveManager
}

func (guangyaLoginStub) BeginGuangyaLogin(context.Context) (drive.GuangyaLoginSession, error) {
	return drive.GuangyaLoginSession{
		ID:        uuid.NewString(),
		QRCodeURL: "https://account.guangyapan.com/__/auth/device/?user_code=user",
	}, nil
}

func (guangyaLoginStub) GuangyaLoginStatus(context.Context, string) (drive.LoginStatus, error) {
	return drive.LoginStatus{State: pan.LoginAuthorized, AccountID: "account-1"}, nil
}

func guangyaLoginRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := gin.New()
	router.Use(errorMiddleware(logger))
	router.POST("/guangya/login", panBeginGuangyaLoginHandler(guangyaLoginStub{}))
	router.GET("/guangya/login/:id", panGuangyaLoginStatusHandler(guangyaLoginStub{}))
	return router
}

func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func getRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestGuangyaLoginHandlersStartAndPoll(t *testing.T) {
	router := guangyaLoginRouter()
	begin := postJSON(router, "/guangya/login", `{}`)
	if begin.Code != http.StatusOK {
		t.Fatalf("begin status = %d, body = %s", begin.Code, begin.Body.String())
	}
	var session drive.GuangyaLoginSession
	if err := json.Unmarshal(begin.Body.Bytes(), &session); err != nil ||
		session.ID == "" || session.QRCodeURL == "" {
		t.Fatalf("begin body = %s, err = %v", begin.Body.String(), err)
	}

	status := getRequest(router, "/guangya/login/"+session.ID)
	if status.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status.Code, status.Body.String())
	}
	var login drive.LoginStatus
	if err := json.Unmarshal(status.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.State != pan.LoginAuthorized || login.AccountID != "account-1" {
		t.Fatalf("login = %+v", login)
	}
}

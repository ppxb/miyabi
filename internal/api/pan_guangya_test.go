package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/drive"
)

type guangyaLoginStub struct {
	DriveManager
}

func (guangyaLoginStub) BeginGuangyaLogin(_ context.Context, phone string) (drive.GuangyaLoginSession, error) {
	return drive.GuangyaLoginSession{ID: uuid.NewString()}, nil
}

func (guangyaLoginStub) VerifyGuangyaLogin(_ context.Context, id, code string) (drive.AccountStatus, error) {
	return drive.AccountStatus{AccountID: id, Provider: drive.ProviderGuangya, Connected: true}, nil
}

func guangyaLoginRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := gin.New()
	router.Use(errorMiddleware(logger))
	router.POST("/guangya/login", panBeginGuangyaLoginHandler(guangyaLoginStub{}))
	router.POST("/guangya/login/:id/verify", panVerifyGuangyaLoginHandler(guangyaLoginStub{}))
	return router
}

func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestGuangyaLoginHandlersAcceptValidJSON(t *testing.T) {
	router := guangyaLoginRouter()
	begin := postJSON(router, "/guangya/login", `{"phone":"+8613800138000"}`)
	if begin.Code != http.StatusOK {
		t.Fatalf("begin status = %d, body = %s", begin.Code, begin.Body.String())
	}
	var session drive.GuangyaLoginSession
	if err := json.Unmarshal(begin.Body.Bytes(), &session); err != nil || session.ID == "" {
		t.Fatalf("begin body = %s, err = %v", begin.Body.String(), err)
	}

	verify := postJSON(router, "/guangya/login/"+session.ID+"/verify", `{"code":"123456"}`)
	if verify.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body = %s", verify.Code, verify.Body.String())
	}
}

func TestGuangyaLoginHandlersReportMissingFields(t *testing.T) {
	router := guangyaLoginRouter()
	begin := postJSON(router, "/guangya/login", `{}`)
	if begin.Code != http.StatusBadRequest || !strings.Contains(begin.Body.String(), "请输入手机号") {
		t.Fatalf("begin = %d %s", begin.Code, begin.Body.String())
	}
	verify := postJSON(router, "/guangya/login/"+uuid.NewString()+"/verify", `{}`)
	if verify.Code != http.StatusBadRequest || !strings.Contains(verify.Body.String(), "请输入验证码") {
		t.Fatalf("verify = %d %s", verify.Code, verify.Body.String())
	}
}

package service

import (
	"errors"
	"testing"
)

func TestAccessGate(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		input    string
		valid    bool
	}{
		{name: "disabled", valid: true},
		{name: "disabled with input", input: "anything", valid: true},
		{name: "correct", password: "test-password", input: "test-password", valid: true},
		{name: "wrong", password: "test-password", input: "wrong"},
		{name: "empty", password: "test-password"},
		{name: "prefix", password: "test-password", input: "test"},
		{name: "case sensitive", password: "test-password", input: "TEST-PASSWORD"},
		{name: "unicode", password: "测试密码", input: "测试密码", valid: true},
		{name: "preserve whitespace", password: " test-password ", input: "test-password"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gate := NewAccessGateService(test.password)
			if gate.Enabled() != (test.password != "") {
				t.Fatal("unexpected gate state")
			}
			err := gate.Verify(test.input)
			if test.valid && err != nil {
				t.Fatalf("verify password: %v", err)
			}
			if !test.valid && !errors.Is(err, ErrAccessPassword) {
				t.Fatalf("expected password error, got %v", err)
			}
		})
	}
}

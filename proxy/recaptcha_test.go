package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyRecaptcha_Pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": true, "score": 0.9})
	}))
	defer srv.Close()

	old := recaptchaVerifyURL
	recaptchaVerifyURL = srv.URL
	defer func() { recaptchaVerifyURL = old }()

	ok, err := verifyRecaptcha("secret", "token", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected pass for score 0.9 with threshold 0.5")
	}
}

func TestVerifyRecaptcha_ScoreBelowThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": true, "score": 0.2})
	}))
	defer srv.Close()

	old := recaptchaVerifyURL
	recaptchaVerifyURL = srv.URL
	defer func() { recaptchaVerifyURL = old }()

	ok, err := verifyRecaptcha("secret", "token", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected fail for score 0.2 with threshold 0.5")
	}
}

func TestVerifyRecaptcha_SuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error-codes": []string{"invalid-input-secret"}})
	}))
	defer srv.Close()

	old := recaptchaVerifyURL
	recaptchaVerifyURL = srv.URL
	defer func() { recaptchaVerifyURL = old }()

	ok, err := verifyRecaptcha("secret", "token", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected fail when success=false")
	}
}

func TestVerifyRecaptcha_NetworkError(t *testing.T) {
	old := recaptchaVerifyURL
	recaptchaVerifyURL = "http://127.0.0.1:1"
	defer func() { recaptchaVerifyURL = old }()

	ok, err := verifyRecaptcha("secret", "token", 0.5)
	if err == nil {
		t.Error("expected error on network failure")
	}
	if ok {
		t.Error("expected false on network failure")
	}
}

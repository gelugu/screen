package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"screen/configuration"
)

func TestAPIMode_OTP_MissingHeader(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp", Mode: "api"}},
	})

	w := get(srv, "example.com", "/data")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}

	var body apiErrorBody
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "otp_required" {
		t.Errorf("error = %q, want otp_required", body.Error)
	}
}

func TestAPIMode_OTP_WrongCode(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp", Mode: "api"}},
	})

	r := httptest.NewRequest("GET", "/data", nil)
	r.Host = "example.com"
	r.Header.Set("X-OTP-Token", "wrong")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}

	var body apiErrorBody
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "otp_invalid" {
		t.Errorf("error = %q, want otp_invalid", body.Error)
	}
}

func TestAPIMode_OTP_CorrectCode_Proxied(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp", Mode: "api"}},
	})

	r := httptest.NewRequest("GET", "/data", nil)
	r.Host = "example.com"
	r.Header.Set("X-OTP-Token", "1234")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("X-Upstream") != "ok" {
		t.Error("expected response from upstream")
	}
}

func TestAPIMode_OTP_SessionReuse(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp", Mode: "api"}},
	})

	// First request: pass OTP header to get session cookie
	r1 := httptest.NewRequest("GET", "/data", nil)
	r1.Host = "example.com"
	r1.Header.Set("X-OTP-Token", "1234")
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", w1.Code)
	}

	// Second request: no header, only session cookie
	w2 := get(srv, "example.com", "/data", cookies(w1)...)
	if w2.Code != http.StatusOK {
		t.Errorf("second request status = %d, want 200 (session reuse)", w2.Code)
	}
}

func TestAPIMode_ContentType_JSON(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp", Mode: "api"}},
	})

	w := get(srv, "example.com", "/data")
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

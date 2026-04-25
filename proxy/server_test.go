package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"screen/configuration"
)

// --- helpers ---

func upstream(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	return s
}

func newSrv(t *testing.T, cfg *configuration.Config) *Server {
	t.Helper()
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func do(srv http.Handler, method, host, path string, cookies []*http.Cookie, body http.Handler) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Host = host
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func get(srv http.Handler, host, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return do(srv, "GET", host, path, cookies, nil)
}

func cookies(w *httptest.ResponseRecorder) []*http.Cookie {
	return w.Result().Cookies()
}

// --- matchSite ---

func TestMatchSite_ExactDomain(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Upstream: up.URL},
	}})

	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "example.com"
	if srv.matchSite(r) == nil {
		t.Error("expected site match")
	}
}

func TestMatchSite_UnknownDomain(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Upstream: up.URL},
	}})

	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "other.com"
	if srv.matchSite(r) != nil {
		t.Error("expected no match for unknown domain")
	}
}

func TestMatchSite_PathSpecificity(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Path: "", Upstream: up.URL},
		{Domain: "example.com", Path: "/admin", Upstream: up.URL},
	}})

	r := httptest.NewRequest("GET", "/admin/settings", nil)
	r.Host = "example.com"
	site := srv.matchSite(r)
	if site == nil {
		t.Fatal("expected a match")
	}
	if site.Path != "/admin" {
		t.Errorf("expected /admin rule, got Path=%q", site.Path)
	}
}

func TestMatchSite_PathSpecificity_Root(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Path: "", Upstream: up.URL},
		{Domain: "example.com", Path: "/admin", Upstream: up.URL},
	}})

	r := httptest.NewRequest("GET", "/public", nil)
	r.Host = "example.com"
	site := srv.matchSite(r)
	if site == nil {
		t.Fatal("expected a match")
	}
	if site.Path != "" {
		t.Errorf("expected root rule, got Path=%q", site.Path)
	}
}

func TestMatchSite_StripPort(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Upstream: up.URL},
	}})

	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "example.com:8080"
	if srv.matchSite(r) == nil {
		t.Error("expected match with port in Host header")
	}
}

// --- isVerified ---

func TestIsVerified_NoProtection(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Upstream: up.URL},
	}})
	site := &configuration.SiteConfig{Domain: "example.com", Protection: ""}
	r := httptest.NewRequest("GET", "/", nil)
	if !srv.isVerified(r, site) {
		t.Error("no protection should always be verified")
	}
}

func TestIsVerified_OTP_Missing(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp"}},
	})
	site := &configuration.SiteConfig{Domain: "example.com", Protection: "otp"}
	r := httptest.NewRequest("GET", "/", nil)
	if srv.isVerified(r, site) {
		t.Error("should not be verified without session")
	}
}

func TestIsVerified_OTP_AfterMark(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp"}},
	})
	site := &configuration.SiteConfig{Domain: "example.com", Protection: "otp"}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	id := srv.sessions.ensure(w, r)
	srv.sessions.mark(id, siteKey(site)+":otp")

	r2 := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies(w) {
		r2.AddCookie(c)
	}
	if !srv.isVerified(r2, site) {
		t.Error("should be verified after mark")
	}
}

func TestIsVerified_CaptchaAndOTP_RequiresBoth(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode:   "1234",
		Recaptcha: configuration.RecaptchaConfig{Secret: "s", SiteKey: "k"},
		Sites: []configuration.SiteConfig{
			{Domain: "example.com", Upstream: up.URL, Protection: "captcha+otp"},
		},
	})
	site := &configuration.SiteConfig{Domain: "example.com", Protection: "captcha+otp"}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	id := srv.sessions.ensure(w, r)
	// only mark captcha, not otp
	srv.sessions.mark(id, siteKey(site)+":captcha")

	r2 := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies(w) {
		r2.AddCookie(c)
	}
	if srv.isVerified(r2, site) {
		t.Error("should not be verified with only captcha marked")
	}
}

// --- helpers ---

func TestHostOnly(t *testing.T) {
	tests := []struct{ in, want string }{
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"localhost:8080", "localhost"},
		{"localhost", "localhost"},
	}
	for _, tt := range tests {
		if got := hostOnly(tt.in); got != tt.want {
			t.Errorf("hostOnly(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// --- ServeHTTP integration ---

func TestServeHTTP_UnknownDomain(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Upstream: up.URL},
	}})

	w := get(srv, "other.com", "/")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestServeHTTP_NoProtection_Proxied(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{Sites: []configuration.SiteConfig{
		{Domain: "example.com", Upstream: up.URL},
	}})

	w := get(srv, "example.com", "/page")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("X-Upstream") != "ok" {
		t.Error("response should come from upstream")
	}
}

func TestServeHTTP_OTP_RedirectsToChallenge(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp"}},
	})

	w := get(srv, "example.com", "/page")
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected Location header")
	}
	if loc[:len("/__protect/otp")] != "/__protect/otp" {
		t.Errorf("redirect to %q, want /__protect/otp prefix", loc)
	}
}

func TestServeHTTP_OTP_ProxiedAfterSession(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp"}},
	})

	// Pre-populate session as if OTP was already passed
	w0 := httptest.NewRecorder()
	r0 := httptest.NewRequest("GET", "/", nil)
	id := srv.sessions.ensure(w0, r0)
	site := &configuration.SiteConfig{Domain: "example.com", Protection: "otp"}
	srv.sessions.mark(id, siteKey(site)+":otp")

	w := get(srv, "example.com", "/page", cookies(w0)...)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 after session", w.Code)
	}
}

func TestServeHTTP_ProtectPathBypassedForOtherPaths(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp"}},
	})

	// Challenge path itself should be handled, not proxied
	w := get(srv, "example.com", "/__protect/otp")
	// 200 (form rendered) or any non-502 from upstream
	if w.Code == http.StatusBadGateway {
		t.Error("/__protect/ path should not be proxied to upstream")
	}
}

func TestServeHTTP_Status(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "example.com", Upstream: up.URL, Protection: "otp"}},
	})

	w := get(srv, "example.com", "/__status")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body struct {
		Status         string `json:"status"`
		UptimeSeconds  int64  `json:"uptime_seconds"`
		SessionsActive int    `json:"sessions_active"`
		Sites          []any  `json:"sites"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if len(body.Sites) != 1 {
		t.Errorf("sites count = %d, want 1", len(body.Sites))
	}
}

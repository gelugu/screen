//go:build loadtest

package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"screen/configuration"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

const (
	loadRate     = 5000
	loadDuration = 5 * time.Second
	maxP99       = 50 * time.Millisecond
)

// wrap starts an httptest.Server that fixes the Host header before handing off
// to the proxy, since vegeta connects via IP and the proxy routes by domain.
func wrap(domain string, srv *Server) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = domain
		srv.ServeHTTP(w, r)
	}))
}

func attack(t *testing.T, url string, header http.Header) *vegeta.Metrics {
	t.Helper()
	target := vegeta.Target{Method: "GET", URL: url}
	if header != nil {
		target.Header = header
	}
	attacker := vegeta.NewAttacker()
	rate := vegeta.Rate{Freq: loadRate, Per: time.Second}

	var m vegeta.Metrics
	for res := range attacker.Attack(vegeta.NewStaticTargeter(target), rate, loadDuration, t.Name()) {
		m.Add(res)
	}
	m.Close()
	return &m
}

func assertMetrics(t *testing.T, m *vegeta.Metrics) {
	t.Helper()
	t.Logf("requests=%-5d  success=%.1f%%  p50=%-8s  p95=%-8s  p99=%-8s  throughput=%.0f req/s",
		m.Requests, m.Success*100,
		m.Latencies.P50, m.Latencies.P95, m.Latencies.P99,
		m.Throughput,
	)
	if m.Success < 1.0 {
		for code, count := range m.StatusCodes {
			if code != "200" {
				t.Errorf("unexpected status %s: %d responses", code, count)
			}
		}
	}
	if p99 := m.Latencies.P99; p99 > maxP99 {
		t.Errorf("p99 %s exceeds threshold %s", p99, maxP99)
	}
}

// TestLoad_NoProtection measures raw proxy throughput with no auth layer.
func TestLoad_NoProtection(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		Sites: []configuration.SiteConfig{
			{Domain: "bench.local", Upstream: up.URL},
		},
	})
	ts := wrap("bench.local", srv)
	t.Cleanup(ts.Close)

	m := attack(t, ts.URL+"/page", nil)
	assertMetrics(t, m)
}

// TestLoad_OTP_VerifiedSession measures throughput when every request already
// carries a valid session cookie (the common hot path after initial auth).
func TestLoad_OTP_VerifiedSession(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "bench.local", Upstream: up.URL, Protection: "otp"}},
	})

	w0 := httptest.NewRecorder()
	r0 := httptest.NewRequest("GET", "/", nil)
	id := srv.sessions.ensure(w0, r0)
	site := &configuration.SiteConfig{Domain: "bench.local", Protection: "otp"}
	srv.sessions.mark(id, siteKey(site)+":otp")

	var cookieHeader string
	for _, c := range w0.Result().Cookies() {
		if c.Name == sessionCookie {
			cookieHeader = fmt.Sprintf("%s=%s", sessionCookie, c.Value)
		}
	}

	ts := wrap("bench.local", srv)
	t.Cleanup(ts.Close)

	m := attack(t, ts.URL+"/page", http.Header{"Cookie": []string{cookieHeader}})
	assertMetrics(t, m)
}

// TestLoad_OTP_ChallengeRedirects measures the redirect path for unverified
// requests — every request hits the challenge gate and gets a 302.
func TestLoad_OTP_ChallengeRedirects(t *testing.T) {
	up := upstream(t)
	srv := newSrv(t, &configuration.Config{
		OTPCode: "1234",
		Sites:   []configuration.SiteConfig{{Domain: "bench.local", Upstream: up.URL, Protection: "otp"}},
	})
	ts := wrap("bench.local", srv)
	t.Cleanup(ts.Close)

	attacker := vegeta.NewAttacker()
	rate := vegeta.Rate{Freq: loadRate, Per: time.Second}
	target := vegeta.Target{Method: "GET", URL: ts.URL + "/page"}

	var m vegeta.Metrics
	for res := range attacker.Attack(vegeta.NewStaticTargeter(target), rate, loadDuration, t.Name()) {
		// 302 is expected here — treat it as success
		if res.Code == http.StatusFound {
			res.Code = http.StatusOK
		}
		m.Add(res)
	}
	m.Close()

	assertMetrics(t, &m)
}

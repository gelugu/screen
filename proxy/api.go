package proxy

import (
	"encoding/json"
	"net/http"
	"screen/configuration"
	"screen/logging"
	"screen/metrics"
)

const (
	headerRecaptchaToken = "X-Recaptcha-Token"
	headerOTPToken       = "X-OTP-Token"
)

var apiLog = logging.NewLogger("api")

// handleAPIRequest verifies API-mode protection headers, marks the session on
// success, and proxies the request. Returns the result string for metrics.
func (s *Server) handleAPIRequest(w http.ResponseWriter, r *http.Request, site *configuration.SiteConfig) string {
	k := siteKey(site)
	rc := s.cfg.Recaptcha

	needsCaptcha := site.Protection == "captcha" || site.Protection == "captcha+otp"
	needsOTP := site.Protection == "otp" || site.Protection == "captcha+otp"

	if needsCaptcha && !s.sessions.verified(r, k+":captcha") {
		token := r.Header.Get(headerRecaptchaToken)
		if token == "" {
			apiLog.Warnf("API missing recaptcha token domain=%s path=%q", site.Domain, r.URL.Path)
			metrics.ChallengesTotal.WithLabelValues(site.Domain, "captcha", "issued").Inc()
			writeAPIError(w, http.StatusUnauthorized, "recaptcha_required",
				"include "+headerRecaptchaToken+" header with a reCAPTCHA v3 token")
			return "challenged"
		}

		ok, err := verifyRecaptcha(rc.Secret, token, rc.Threshold)
		if err != nil || !ok {
			apiLog.Warnf("API recaptcha failed domain=%s path=%q err=%v", site.Domain, r.URL.Path, err)
			metrics.ChallengesTotal.WithLabelValues(site.Domain, "captcha", "failed").Inc()
			writeAPIError(w, http.StatusUnauthorized, "recaptcha_failed",
				"reCAPTCHA verification failed or score below threshold")
			return "challenged"
		}

		id := s.sessions.ensure(w, r)
		s.sessions.mark(id, k+":captcha")
		metrics.ChallengesTotal.WithLabelValues(site.Domain, "captcha", "passed").Inc()
		apiLog.Infof("API captcha verified domain=%s session=%s", site.Domain, id[:8])
	}

	if needsOTP && !s.sessions.verified(r, k+":otp") {
		code := r.Header.Get(headerOTPToken)
		if code == "" {
			apiLog.Warnf("API missing OTP token domain=%s path=%q", site.Domain, r.URL.Path)
			metrics.ChallengesTotal.WithLabelValues(site.Domain, "otp", "issued").Inc()
			writeAPIError(w, http.StatusUnauthorized, "otp_required",
				"include "+headerOTPToken+" header with the access code")
			return "challenged"
		}
		if code != s.cfg.OTPCode {
			apiLog.Warnf("API OTP invalid domain=%s path=%q", site.Domain, r.URL.Path)
			metrics.ChallengesTotal.WithLabelValues(site.Domain, "otp", "failed").Inc()
			writeAPIError(w, http.StatusUnauthorized, "otp_invalid", "invalid OTP code")
			return "challenged"
		}

		id := s.sessions.ensure(w, r)
		s.sessions.mark(id, k+":otp")
		metrics.ChallengesTotal.WithLabelValues(site.Domain, "otp", "passed").Inc()
		apiLog.Infof("API OTP verified domain=%s session=%s", site.Domain, id[:8])
	}

	apiLog.Debugf("API request verified, forwarding to %s", site.Upstream)
	s.proxies[site.Upstream].ServeHTTP(w, r)
	return "proxied"
}

type apiErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiErrorBody{Error: code, Message: message})
}

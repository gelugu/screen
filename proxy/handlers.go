package proxy

import (
	"net/http"
	"net/url"
	"screen/logging"
	"screen/metrics"
)

var handlerLog = logging.NewLogger("handlers")

type pageData struct {
	Domain           string
	Path             string
	Back             string
	Error            bool
	RecaptchaSiteKey string
}

func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/__protect/otp":
		if r.Method == http.MethodGet {
			s.otpForm(w, r)
		} else if r.Method == http.MethodPost {
			s.otpSubmit(w, r)
		}
	case "/__protect/captcha":
		if r.Method == http.MethodGet {
			s.captchaForm(w, r)
		} else if r.Method == http.MethodPost {
			s.captchaSubmit(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) otpForm(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domain := q.Get("d")
	path := q.Get("p")
	hasError := q.Get("err") == "1"

	handlerLog.Debugf("serving OTP form domain=%s path=%q error=%v", domain, path, hasError)

	data := pageData{
		Domain: domain,
		Path:   path,
		Back:   safeBack(q.Get("back")),
		Error:  hasError,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.otpTmpl.Execute(w, data); err != nil {
		handlerLog.Errorf("failed to render OTP template: %v", err)
	}
}

func (s *Server) otpSubmit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	domain := r.FormValue("d")
	path := r.FormValue("p")
	back := safeBack(r.FormValue("back"))
	code := r.FormValue("code")

	site := s.findSite(domain, path)
	if site == nil {
		handlerLog.Errorf("OTP submit for unknown site domain=%s path=%q", domain, path)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if code != s.cfg.OTPCode {
		handlerLog.Warnf("OTP failed domain=%s path=%q", domain, path)
		metrics.ChallengesTotal.WithLabelValues(domain, "otp", "failed").Inc()
		q := url.Values{}
		q.Set("d", domain)
		q.Set("p", path)
		q.Set("back", back)
		q.Set("err", "1")
		http.Redirect(w, r, "/__protect/otp?"+q.Encode(), http.StatusFound)
		return
	}

	id := s.sessions.ensure(w, r)
	s.sessions.mark(id, siteKey(site)+markOTP)
	metrics.ChallengesTotal.WithLabelValues(domain, "otp", "passed").Inc()
	handlerLog.Infof("OTP verified domain=%s path=%q session=%s", domain, path, id[:8])
	http.Redirect(w, r, back, http.StatusFound)
}

func (s *Server) captchaForm(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domain := q.Get("d")
	path := q.Get("p")
	hasError := q.Get("err") == "1"

	handlerLog.Debugf("serving captcha form domain=%s path=%q error=%v", domain, path, hasError)

	data := pageData{
		Domain:           domain,
		Path:             path,
		Back:             safeBack(q.Get("back")),
		Error:            hasError,
		RecaptchaSiteKey: s.cfg.Recaptcha.SiteKey,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.captchaTmpl.Execute(w, data); err != nil {
		handlerLog.Errorf("failed to render captcha template: %v", err)
	}
}

func (s *Server) captchaSubmit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	domain := r.FormValue("d")
	path := r.FormValue("p")
	back := safeBack(r.FormValue("back"))
	token := r.FormValue("g-recaptcha-response")

	site := s.findSite(domain, path)
	if site == nil {
		handlerLog.Errorf("captcha submit for unknown site domain=%s path=%q", domain, path)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if token == "" {
		handlerLog.Warnf("captcha submit with empty token domain=%s path=%q", domain, path)
		q := url.Values{}
		q.Set("d", domain)
		q.Set("p", path)
		q.Set("back", back)
		q.Set("err", "1")
		http.Redirect(w, r, "/__protect/captcha?"+q.Encode(), http.StatusFound)
		return
	}

	rc := s.cfg.Recaptcha
	ok, err := verifyRecaptcha(rc.Secret, token, rc.Threshold)
	if err != nil || !ok {
		handlerLog.Warnf("captcha failed domain=%s path=%q err=%v", domain, path, err)
		metrics.ChallengesTotal.WithLabelValues(domain, "captcha", "failed").Inc()
		q := url.Values{}
		q.Set("d", domain)
		q.Set("p", path)
		q.Set("back", back)
		q.Set("err", "1")
		http.Redirect(w, r, "/__protect/captcha?"+q.Encode(), http.StatusFound)
		return
	}

	id := s.sessions.ensure(w, r)
	s.sessions.mark(id, siteKey(site)+markCaptcha)
	metrics.ChallengesTotal.WithLabelValues(domain, "captcha", "passed").Inc()
	handlerLog.Infof("captcha verified domain=%s path=%q session=%s", domain, path, id[:8])
	http.Redirect(w, r, back, http.StatusFound)
}

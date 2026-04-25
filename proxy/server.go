package proxy

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"
	"screen/configuration"
	"screen/logging"
	"screen/metrics"
	"strings"
	"time"
)

const (
	markCaptcha   = ":captcha"
	markOTP       = ":otp"
	challengePath = "/__protect/captcha"
	otpPath       = "/__protect/otp"
)

var serverLog = logging.NewLogger("server")

type Server struct {
	cfg         *configuration.Config
	sessions    sessionStore
	proxies     map[string]*httputil.ReverseProxy
	otpTmpl     *template.Template
	captchaTmpl *template.Template
	startTime   time.Time
}

func NewServer(cfg *configuration.Config) (*Server, error) {
	otpTmpl, err := template.ParseFS(templateFS, "templates/otp.html")
	if err != nil {
		return nil, err
	}
	captchaTmpl, err := template.ParseFS(templateFS, "templates/captcha.html")
	if err != nil {
		return nil, err
	}

	sessions, err := newSessionStoreFromConfig(cfg.Redis)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:         cfg,
		sessions:    sessions,
		proxies:     make(map[string]*httputil.ReverseProxy),
		otpTmpl:     otpTmpl,
		captchaTmpl: captchaTmpl,
		startTime:   time.Now(),
	}

	for _, site := range cfg.Sites {
		if _, ok := s.proxies[site.Upstream]; ok {
			continue
		}
		u, err := url.Parse(site.Upstream)
		if err != nil {
			return nil, err
		}
		s.proxies[site.Upstream] = makeProxy(u)
		serverLog.Debugf("registered upstream %s", site.Upstream)
	}

	serverLog.Infof("configured %d site(s)", len(cfg.Sites))
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	domain := hostOnly(r.Host)
	result := "proxied"

	defer func() {
		metrics.RequestDuration.WithLabelValues(domain, r.Method).Observe(time.Since(start).Seconds())
		metrics.RequestsTotal.WithLabelValues(domain, r.Method, result).Inc()
	}()

	serverLog.Debugf("%s %s%s", r.Method, r.Host, r.URL.Path)

	if r.URL.Path == "/__status" {
		result = "status"
		s.handleStatus(w)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/__protect/") {
		result = "challenge"
		s.handleChallenge(w, r)
		return
	}

	site := s.matchSite(r)
	if site == nil {
		result = "not_found"
		serverLog.Warnf("no site matched for host %q", r.Host)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	serverLog.Debugf("matched site domain=%s path=%q upstream=%s", site.Domain, site.Path, site.Upstream)

	if !s.isVerified(r, site) {
		if site.Mode == "api" {
			result = s.handleAPIRequest(w, r, site)
		} else {
			result = "challenged"
			serverLog.Infof("challenge required: domain=%s path=%q protection=%s", site.Domain, site.Path, site.Protection)
			s.redirectToChallenge(w, r, site)
		}
		return
	}

	serverLog.Debugf("forwarding to %s", site.Upstream)
	s.proxies[site.Upstream].ServeHTTP(w, r)
}

func (s *Server) matchSite(r *http.Request) *configuration.SiteConfig {
	host := hostOnly(r.Host)

	var best *configuration.SiteConfig
	bestLen := -1
	for i := range s.cfg.Sites {
		site := &s.cfg.Sites[i]
		if site.Domain != host {
			continue
		}
		if site.Path != "" && !strings.HasPrefix(r.URL.Path, site.Path) {
			continue
		}
		if len(site.Path) > bestLen {
			bestLen = len(site.Path)
			best = site
		}
	}
	return best
}

func (s *Server) isVerified(r *http.Request, site *configuration.SiteConfig) bool {
	k := siteKey(site)
	switch site.Protection {
	case "captcha":
		return s.sessions.verified(r, k+markCaptcha)
	case "otp":
		return s.sessions.verified(r, k+markOTP)
	case "captcha+otp":
		return s.sessions.verified(r, k+markCaptcha) && s.sessions.verified(r, k+markOTP)
	}
	return true
}

func (s *Server) redirectToChallenge(w http.ResponseWriter, r *http.Request, site *configuration.SiteConfig) {
	k := siteKey(site)
	q := url.Values{}
	q.Set("d", site.Domain)
	q.Set("p", site.Path)
	q.Set("back", s.sessions.storeBack(r.URL.RequestURI()))

	var path string
	switch site.Protection {
	case "captcha":
		path = challengePath
		metrics.ChallengesTotal.WithLabelValues(site.Domain, "captcha", "issued").Inc()
	case "otp":
		path = otpPath
		metrics.ChallengesTotal.WithLabelValues(site.Domain, "otp", "issued").Inc()
	case "captcha+otp":
		if !s.sessions.verified(r, k+markCaptcha) {
			path = challengePath
			metrics.ChallengesTotal.WithLabelValues(site.Domain, "captcha", "issued").Inc()
		} else {
			path = otpPath
			metrics.ChallengesTotal.WithLabelValues(site.Domain, "otp", "issued").Inc()
		}
	}

	http.Redirect(w, r, path+"?"+q.Encode(), http.StatusFound)
}

func (s *Server) findSite(domain, path string) *configuration.SiteConfig {
	for i := range s.cfg.Sites {
		site := &s.cfg.Sites[i]
		if site.Domain == domain && site.Path == path {
			return site
		}
	}
	return nil
}

func siteKey(site *configuration.SiteConfig) string {
	return site.Domain + "|" + site.Path
}

func hostOnly(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}

func (s *Server) handleStatus(w http.ResponseWriter) {
	type siteStatus struct {
		Domain     string `json:"domain"`
		Path       string `json:"path,omitempty"`
		Protection string `json:"protection,omitempty"`
		Mode       string `json:"mode,omitempty"`
		Upstream   string `json:"upstream"`
	}
	type response struct {
		Status         string       `json:"status"`
		UptimeSeconds  int64        `json:"uptime_seconds"`
		SessionsActive int          `json:"sessions_active"`
		Sites          []siteStatus `json:"sites"`
	}

	sites := make([]siteStatus, len(s.cfg.Sites))
	for i, site := range s.cfg.Sites {
		sites[i] = siteStatus{
			Domain:     site.Domain,
			Path:       site.Path,
			Protection: site.Protection,
			Mode:       site.Mode,
			Upstream:   site.Upstream,
		}
	}

	body := response{
		Status:         "ok",
		UptimeSeconds:  int64(time.Since(s.startTime).Seconds()),
		SessionsActive: s.sessions.count(),
		Sites:          sites,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

func makeProxy(target *url.URL) *httputil.ReverseProxy {
	p := httputil.NewSingleHostReverseProxy(target)
	upstream := target.String()
	host := target.Host
	orig := p.Director
	p.Director = func(req *http.Request) {
		orig(req)
		req.Host = host
	}
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		serverLog.Errorf("upstream %s error: %v", host, err)
		metrics.UpstreamErrorsTotal.WithLabelValues(upstream).Inc()
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	return p
}

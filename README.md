# screen

[![CI](https://github.com/gelugu/screen/actions/workflows/ci.yaml/badge.svg)](https://github.com/gelugu/screen/actions/workflows/ci.yaml)
[![Load Tests](https://github.com/gelugu/screen/actions/workflows/loadtest.yaml/badge.svg)](https://github.com/gelugu/screen/actions/workflows/loadtest.yaml)
[![Docker](https://img.shields.io/docker/v/gelugu/screen?label=docker&sort=semver)](https://hub.docker.com/r/gelugu/screen)
[![Go](https://img.shields.io/badge/go-1.24-blue)](https://go.dev)

[![Quality gate](https://sonarcloud.io/api/project_badges/quality_gate?project=gelugu_screen)](https://sonarcloud.io/summary/new_code?id=gelugu_screen)

A lightweight reverse proxy that sits in front of any number of upstream services and gates access with bot protection (reCAPTCHA v3) and OTP authentication — configured in a single YAML file, deployed as a single binary.

```
  browser / API client
        │
        ▼
  ┌──────────────────┐
  │   screen proxy   │  ← one instance, all sites
  │                  │
  │  ┌────────────┐  │
  │  │  captcha   │  │  reCAPTCHA v3 invisible scan
  │  │    otp     │  │  hardcoded code (browser form / API header)
  │  └────────────┘  │
  └─────────┬────────┘
            │ verified session cookie
            ▼
  ┌──────────────────────────────────┐
  │  upstream A │ upstream B │ ...   │
  └──────────────────────────────────┘
```

---

## Table of contents

1. [Protection modes](#protection-modes)
2. [Installation](#installation)
3. [Configuration reference](#configuration-reference)
4. [reCAPTCHA v3 setup](#recaptcha-v3-setup)
5. [Protecting your sites](#protecting-your-sites)
6. [Browser challenge flows](#browser-challenge-flows)
7. [API mode](#api-mode)
8. [Status endpoint](#status-endpoint)
9. [Running](#running)
10. [Local testing](#local-testing)
11. [Load testing](#load-testing)
12. [Metrics](#metrics)
13. [Logs](#logs)

---

## Protection modes

| Mode | Behaviour |
|---|---|
| `captcha` | reCAPTCHA v3 invisible scan. Browser: auto-submit interstitial. API: token in header. |
| `otp` | Access code check. Browser: form. API: code in header. |
| `captcha+otp` | Both in sequence — captcha first, then OTP. |

Protection is delivered differently depending on the site's `mode`:

| `mode` | How challenges are delivered |
|---|---|
| `browser` (default) | HTML pages — redirect to captcha interstitial or OTP form |
| `api` | JSON `401` responses — client sends tokens in request headers |

Once a visitor passes, a session cookie is set and subsequent requests pass through without re-verification.

---

## Installation

**Prerequisites:** Go 1.21+

```bash
git clone <repo>
cd screen
go build -o screen .
```

Cross-compile for Linux:

```bash
GOOS=linux GOARCH=amd64 go build -o screen .
```

---

## Configuration reference

Pass the config file path via `--config-path` (default: `/etc/screen/config.yaml`).

```yaml
log_level: info          # debug | info | warn | error
port: 8080               # proxy listening port
metrics_port: 9090       # Prometheus metrics port; set 0 to disable

otp: ""                  # access code shared across all OTP-protected sites

recaptcha:
  secret: ""             # reCAPTCHA v3 secret key (server-side, keep private)
  site_key: ""           # reCAPTCHA v3 site key (browser-side, public)
  threshold: 0.5         # score cutoff 0.0–1.0; higher = stricter

sites:
  - domain: example.com  # exact Host header to match
    path: ""             # optional path prefix; most specific wins
    upstream: http://... # where to forward verified requests
    protection: captcha  # captcha | otp | captcha+otp | (empty = no protection)
    mode: browser        # browser (default) | api
```

### All fields

| Field | Default | Description |
|---|---|---|
| `log_level` | `info` | Logrus level |
| `port` | `8080` | Proxy HTTP port |
| `metrics_port` | `9090` | Prometheus `/metrics` port; `0` disables |
| `otp` | — | Access code for OTP challenges (required if any site uses otp) |
| `recaptcha.secret` | — | reCAPTCHA v3 **secret key** (required if any site uses captcha) |
| `recaptcha.site_key` | — | reCAPTCHA v3 **site key** (required if any site uses captcha) |
| `recaptcha.threshold` | `0.5` | Minimum score to pass (0.0–1.0) |
| `sites[].domain` | required | Exact `Host` header value |
| `sites[].path` | `""` | Path prefix; empty matches all paths on that domain |
| `sites[].upstream` | required | URL of the upstream service |
| `sites[].protection` | `""` | Protection mode; empty = proxy directly |
| `sites[].mode` | `browser` | Challenge delivery: `browser` or `api` |

### Environment variable overrides

Any config key can be overridden with an env var. Replace `.` with `_` and uppercase:

```bash
LOG_LEVEL=debug PORT=9000 ./screen --config-path config.yaml
RECAPTCHA_SECRET=xyz RECAPTCHA_SITE_KEY=abc ./screen --config-path config.yaml
```

### Site matching

When a request arrives, screen picks the **most specific** rule — longest `path` prefix that matches the `domain`.

```
Request: app.example.com /admin/settings

Rules:
  domain=app.example.com  path=""       → matches (score 0)
  domain=app.example.com  path=/admin   → matches (score 6)  ← winner
  domain=app.example.com  path=/other   → no match
```

---

## reCAPTCHA v3 setup

> **Important:** screen uses **classic reCAPTCHA v3**, not reCAPTCHA Enterprise.
> These are two separate Google products with different consoles.
> reCAPTCHA Enterprise (Google Cloud Console) does not have a secret key — it uses service account credentials. If you're looking in GCP Console, you're in the wrong place.

### Step by step

1. Go to **[google.com/recaptcha/admin/create](https://www.google.com/recaptcha/admin/create)**

2. Fill in the form:
   - **Label**: any name for your reference
   - **reCAPTCHA type**: select **reCAPTCHA v3**
   - **Domains**: add every domain screen will serve (e.g. `example.com`, `app.example.com`)

3. Click **Submit**. On the next screen:
   - **Site Key** → copy to `recaptcha.site_key` (public, safe in HTML)
   - **Secret Key** → copy to `recaptcha.secret` (**never expose this**)

4. Adjust `recaptcha.threshold` based on your traffic:

   | Score | Meaning |
   |---|---|
   | 0.9 | Almost certainly human |
   | 0.5 | Default — good starting point |
   | 0.3 | Lenient — only obvious bots blocked |

   Start at `0.5`. Lower it if real users are getting blocked; raise it if bots slip through.

---

## Protecting your sites

### Landing page — captcha only

```yaml
recaptcha:
  secret: YOUR_SECRET
  site_key: YOUR_SITE_KEY

sites:
  - domain: landing.example.com
    upstream: http://localhost:3001
    protection: captcha
```

### Web app with OTP-gated admin

```yaml
otp: "123456"

sites:
  - domain: app.example.com
    upstream: http://localhost:3002
    protection: captcha

  - domain: app.example.com
    path: /admin
    upstream: http://localhost:3002
    protection: captcha+otp
```

### Plain proxy (no protection)

```yaml
sites:
  - domain: internal.example.com
    upstream: http://localhost:4000
```

---

## Browser challenge flows

### captcha

```
browser → GET /page
        ← 302 /__protect/captcha?back=/page
browser → GET /__protect/captcha        (interstitial loads)
          JS runs reCAPTCHA v3 silently
          form auto-submits with token
server verifies token score ≥ threshold
        ← 302 /page  (session cookie set)
browser → GET /page                     (proxied to upstream)
```

On failure, the user sees a "BLOCKED" screen with a retry button.

### otp

```
browser → GET /page
        ← 302 /__protect/otp?back=/page
browser → GET /__protect/otp            (code form)
user types code → POST /__protect/otp
server checks code == config otp
        ← 302 /page  (session cookie set)
browser → GET /page                     (proxied to upstream)
```

### captcha+otp

Captcha runs first. After it passes the OTP form is shown. After OTP passes the user reaches the original URL. Each step is stored separately in the session.

---

## API mode

When `mode: api` is set, screen returns `401` JSON instead of HTML redirects. Once a client provides valid tokens a session cookie is set — subsequent requests pass through without re-sending tokens.

### Configuration

```yaml
sites:
  - domain: api.example.com
    upstream: http://localhost:3002
    protection: captcha
    mode: api
```

### Headers

| Protection | Header | Value |
|---|---|---|
| `captcha` | `X-Recaptcha-Token` | reCAPTCHA v3 token from `grecaptcha.execute()` |
| `otp` | `X-OTP-Token` | The configured OTP code |
| `captcha+otp` | both | Both must be present and valid |

### 401 response format

```json
{
  "error": "recaptcha_required",
  "message": "include X-Recaptcha-Token header with a reCAPTCHA v3 token"
}
```

| `error` value | Meaning |
|---|---|
| `recaptcha_required` | `X-Recaptcha-Token` header missing |
| `recaptcha_failed` | Token invalid or score below threshold |
| `otp_required` | `X-OTP-Token` header missing |
| `otp_invalid` | Wrong OTP code |

### Getting a reCAPTCHA token in the browser

```html
<script src="https://www.google.com/recaptcha/api.js?render=YOUR_SITE_KEY"></script>
```

```js
async function fetchWithCaptcha(url, options = {}) {
  const token = await new Promise(resolve =>
    grecaptcha.ready(() =>
      grecaptcha.execute('YOUR_SITE_KEY', { action: 'api' }).then(resolve)
    )
  );
  return fetch(url, {
    ...options,
    headers: { ...options.headers, 'X-Recaptcha-Token': token },
  });
}
```

> reCAPTCHA v3 tokens expire in ~2 minutes and can only be verified once. After the first successful request the session cookie means you don't need to send the token again.

### curl examples

```bash
# OTP-only API
curl -H "X-OTP-Token: 123456" https://api.example.com/data

# captcha — first request (token from browser JS, stored in session)
curl -c cookies.txt \
  -H "X-Recaptcha-Token: $TOKEN" \
  https://api.example.com/data

# subsequent requests — session cookie carries the verification
curl -b cookies.txt https://api.example.com/data
```

---

## Status endpoint

`GET /__status` — available on the proxy port on any domain. Not protected.

```bash
curl http://localhost:8080/__status
```

```json
{
  "status": "ok",
  "uptime_seconds": 142,
  "sessions_active": 3,
  "sites": [
    { "domain": "app.example.com", "protection": "captcha", "upstream": "http://localhost:3002" },
    { "domain": "app.example.com", "path": "/admin", "protection": "captcha+otp", "mode": "browser", "upstream": "http://localhost:3002" }
  ]
}
```

Useful for load balancer health checks and quick operational checks.

---

## Running

```bash
# default config path /etc/screen/config.yaml
./screen

# custom config
./screen --config-path /path/to/config.yaml

# env overrides (key → KEY, dots → underscores)
LOG_LEVEL=debug PORT=9000 ./screen --config-path config.yaml
```

### Behind nginx

screen routes by `Host` header — pass it through unchanged:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

---

## Local testing

You can test the full flow locally without a domain or reCAPTCHA keys by using OTP-only protection and `localhost` as the domain.

### 1. Start a dummy upstream

```bash
# Python's built-in HTTP server serves the current directory
python3 -m http.server 3001
```

### 2. Create a local config

```yaml
# local.yaml
log_level: debug
port: 8080
metrics_port: 9090

otp: "1234"

sites:
  - domain: localhost
    upstream: http://localhost:3001
    protection: otp
```

### 3. Run screen

```bash
./screen --config-path local.yaml
```

### 4. Test with a browser

Open `http://localhost:8080` — you'll be redirected to the OTP form. Enter `1234`. You'll land on the upstream.

### 5. Test with curl

```bash
# First request — gets redirected to OTP challenge
curl -v http://localhost:8080/

# Check status
curl http://localhost:8080/__status

# Browser OTP flow — follow redirect, post code, follow again
curl -c jar.txt -L http://localhost:8080/   # redirects to OTP form (302)
curl -c jar.txt -b jar.txt -X POST \
  -d "d=localhost&p=&back=/&code=1234" \
  http://localhost:8080/__protect/otp       # submits code, sets session cookie
curl -c jar.txt -b jar.txt http://localhost:8080/  # now proxied to upstream

# API mode
curl -H "X-OTP-Token: 1234" http://localhost:8080/
```

### 6. Testing API mode locally

Add a second site entry with `mode: api`:

```yaml
otp: "1234"

sites:
  - domain: localhost
    upstream: http://localhost:3001
    protection: otp
    mode: api
```

```bash
# Missing token → 401
curl -s http://localhost:8080/ | python3 -m json.tool
# {"error": "otp_required", "message": "include X-OTP-Token header with the access code"}

# Wrong token → 401
curl -s -H "X-OTP-Token: wrong" http://localhost:8080/ | python3 -m json.tool

# Correct token → proxied, session cookie set
curl -c jar.txt -H "X-OTP-Token: 1234" http://localhost:8080/

# Subsequent request without header — session cookie carries it
curl -b jar.txt http://localhost:8080/
```

### 7. Check metrics

```bash
curl http://localhost:9090/metrics | grep screen_
```

---

## Load testing

Load tests live in `proxy/load_test.go` behind the `loadtest` build tag so they never run during regular `go test ./...`.

```bash
go test -tags loadtest -run TestLoad -v -timeout 120s ./proxy/
```

Three scenarios, each running at 100 req/s for 5 seconds (500 requests total):

| Test | What it measures |
|---|---|
| `TestLoad_NoProtection` | Raw proxy throughput — no auth, just reverse-proxy overhead |
| `TestLoad_OTP_VerifiedSession` | Hot path — every request carries a valid session cookie |
| `TestLoad_OTP_ChallengeRedirects` | Challenge gate — every request hits the 302 redirect path |

Each test logs a one-line summary and fails if p99 latency exceeds 50 ms or any request returns an unexpected status code:

```
load_test.go:79: requests=500   success=100.0%  p50=792µs   p95=1.2ms   p99=2.5ms   throughput=100 req/s
```

Load tests run automatically on every push to `main` in a separate workflow, in parallel with the unit test job.

---

## Metrics

screen exposes Prometheus metrics at `http://localhost:{metrics_port}/metrics` (default port `9090`). Set `metrics_port: 0` to disable.

### Available metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `screen_requests_total` | Counter | `domain`, `method`, `result` | All requests; result: `proxied`\|`challenged`\|`not_found`\|`challenge`\|`status` |
| `screen_request_duration_seconds` | Histogram | `domain`, `method` | End-to-end handling time |
| `screen_challenges_total` | Counter | `domain`, `type`, `result` | Challenge events; type: `captcha`\|`otp`; result: `issued`\|`passed`\|`failed` |
| `screen_recaptcha_score` | Histogram | `result` | reCAPTCHA v3 score distribution; result: `pass`\|`fail` |
| `screen_recaptcha_errors_total` | Counter | — | Errors communicating with the reCAPTCHA API |
| `screen_upstream_errors_total` | Counter | `upstream` | Errors from upstream services |
| `screen_sessions_created_total` | Counter | — | Sessions created since startup |
| `screen_sessions_active` | Gauge | — | Current in-memory session count |

Plus standard `go_*` and `process_*` collectors.

### Prometheus scrape config

```yaml
scrape_configs:
  - job_name: screen
    static_configs:
      - targets: ["localhost:9090"]
```

---

## Logs

| Level | What is logged |
|---|---|
| `debug` | Every request, site match, session lookups, reCAPTCHA scores, API header checks |
| `info` | Startup, config loaded, challenge redirects issued, verifications passed |
| `warn` | Unknown domain, failed OTP/captcha attempts, low scores, missing tokens |
| `error` | Upstream errors, template failures, reCAPTCHA API errors |

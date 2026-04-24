package proxy

import (
	"encoding/json"
	"net/http"
	"net/url"
	"screen/logging"
	"screen/metrics"
)

var recaptchaLog = logging.NewLogger("recaptcha")

var recaptchaVerifyURL = "https://www.google.com/recaptcha/api/siteverify"

func verifyRecaptcha(secret, token string, threshold float64) (bool, error) {
	recaptchaLog.Debug("calling recaptcha v3 siteverify")

	resp, err := http.PostForm(recaptchaVerifyURL, url.Values{
		"secret":   {secret},
		"response": {token},
	})
	if err != nil {
		recaptchaLog.Errorf("recaptcha HTTP request failed: %v", err)
		metrics.RecaptchaErrorsTotal.Inc()
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Success    bool     `json:"success"`
		Score      float64  `json:"score"`
		Action     string   `json:"action"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		recaptchaLog.Errorf("failed to decode recaptcha response: %v", err)
		metrics.RecaptchaErrorsTotal.Inc()
		return false, err
	}

	if !result.Success {
		recaptchaLog.Warnf("recaptcha rejected: %v", result.ErrorCodes)
		metrics.RecaptchaErrorsTotal.Inc()
		return false, nil
	}

	pass := result.Score >= threshold
	scoreLabel := "pass"
	if !pass {
		scoreLabel = "fail"
	}

	metrics.RecaptchaScores.WithLabelValues(scoreLabel).Observe(result.Score)
	recaptchaLog.Debugf("recaptcha score=%.2f threshold=%.2f action=%s result=%s",
		result.Score, threshold, result.Action, scoreLabel)

	if !pass {
		recaptchaLog.Warnf("recaptcha score %.2f below threshold %.2f", result.Score, threshold)
	}

	return pass, nil
}

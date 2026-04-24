package configuration

import "testing"

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid — no protection",
			cfg: &Config{
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001"},
				},
			},
		},
		{
			name: "valid — otp only",
			cfg: &Config{
				OTPCode: "1234",
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "otp"},
				},
			},
		},
		{
			name: "valid — captcha with keys",
			cfg: &Config{
				Recaptcha: RecaptchaConfig{Secret: "sec", SiteKey: "key"},
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "captcha"},
				},
			},
		},
		{
			name: "valid — captcha+otp with keys",
			cfg: &Config{
				OTPCode:   "1234",
				Recaptcha: RecaptchaConfig{Secret: "sec", SiteKey: "key"},
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "captcha+otp"},
				},
			},
		},
		{
			name: "valid — api mode",
			cfg: &Config{
				OTPCode: "x",
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "otp", Mode: "api"},
				},
			},
		},
		{
			name:    "missing domain",
			cfg:     &Config{Sites: []SiteConfig{{Upstream: "http://localhost:3001"}}},
			wantErr: true,
		},
		{
			name:    "missing upstream",
			cfg:     &Config{Sites: []SiteConfig{{Domain: "example.com"}}},
			wantErr: true,
		},
		{
			name: "invalid protection value",
			cfg: &Config{
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "magic"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid mode value",
			cfg: &Config{
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Mode: "html"},
				},
			},
			wantErr: true,
		},
		{
			name: "captcha without recaptcha keys",
			cfg: &Config{
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "captcha"},
				},
			},
			wantErr: true,
		},
		{
			name: "captcha+otp without recaptcha keys",
			cfg: &Config{
				OTPCode: "1234",
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "captcha+otp"},
				},
			},
			wantErr: true,
		},
		{
			name: "captcha — missing secret only",
			cfg: &Config{
				Recaptcha: RecaptchaConfig{SiteKey: "key"},
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "captcha"},
				},
			},
			wantErr: true,
		},
		{
			name: "otp without otp code",
			cfg: &Config{
				Sites: []SiteConfig{
					{Domain: "example.com", Upstream: "http://localhost:3001", Protection: "otp"},
				},
			},
			wantErr: true,
		},
		{
			name:    "empty sites list",
			cfg:     &Config{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

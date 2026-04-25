package configuration

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var v = viper.New()

type Config struct {
	LogLevel    string          `mapstructure:"log_level"`
	Port        int             `mapstructure:"port"`
	MetricsPort int             `mapstructure:"metrics_port"`
	OTPCode     string          `mapstructure:"otp"`
	Recaptcha   RecaptchaConfig `mapstructure:"recaptcha"`
	Sites       []SiteConfig    `mapstructure:"sites"`
}

type RecaptchaConfig struct {
	Secret    string  `mapstructure:"secret"`
	SiteKey   string  `mapstructure:"site_key"`
	Threshold float64 `mapstructure:"threshold"`
}

type SiteConfig struct {
	Domain     string `mapstructure:"domain"`
	Path       string `mapstructure:"path"`
	Upstream   string `mapstructure:"upstream"`
	Protection string `mapstructure:"protection"` // captcha | otp | captcha+otp
	Mode       string `mapstructure:"mode"`       // browser (default) | api
}

const (
	protectionCaptcha    = "captcha"
	protectionOTP        = "otp"
	protectionCaptchaOTP = "captcha+otp"
)

var (
	config *Config
	log    = logrus.New().WithField("package", "configuration")
)

func LoadConfig() *Config {
	v.SetConfigFile(configPath())

	v.SetDefault("log_level", "info")
	v.SetDefault("port", 8080)
	v.SetDefault("metrics_port", 9090)
	v.SetDefault("otp", "")

	v.SetDefault("recaptcha.secret", "")
	v.SetDefault("recaptcha.site_key", "")
	v.SetDefault("recaptcha.threshold", 0.5)

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		log.Infof("no config file at %s, using defaults and env", configPath())
	} else {
		log.Infof("loaded config from %s", v.ConfigFileUsed())
	}

	if err := v.Unmarshal(&config); err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}

	if err := validateConfig(config); err != nil {
		log.Fatalf("config validation error: %v", err)
	}

	return config
}

func validateConfig(cfg *Config) error {
	needsCaptcha := false
	needsOTP := false
	for i, site := range cfg.Sites {
		nc, no, err := validateSite(i, site)
		if err != nil {
			return err
		}
		needsCaptcha = needsCaptcha || nc
		needsOTP = needsOTP || no
	}
	if needsCaptcha && (cfg.Recaptcha.Secret == "" || cfg.Recaptcha.SiteKey == "") {
		return fmt.Errorf("recaptcha.secret and recaptcha.site_key are required when any site uses captcha protection")
	}
	if needsOTP && cfg.OTPCode == "" {
		return fmt.Errorf("otp is required when any site uses otp protection")
	}
	return nil
}

func validateSite(i int, site SiteConfig) (needsCaptcha, needsOTP bool, err error) {
	if site.Domain == "" {
		return false, false, fmt.Errorf("sites[%d]: domain is required", i)
	}
	if site.Upstream == "" {
		return false, false, fmt.Errorf("sites[%d]: upstream is required", i)
	}
	switch site.Protection {
	case protectionCaptcha, protectionOTP, protectionCaptchaOTP, "":
	default:
		return false, false, fmt.Errorf("sites[%d]: unknown protection %q", i, site.Protection)
	}
	switch site.Mode {
	case "browser", "api", "":
	default:
		return false, false, fmt.Errorf("sites[%d]: unknown mode %q (browser | api)", i, site.Mode)
	}
	needsCaptcha = site.Protection == protectionCaptcha || site.Protection == protectionCaptchaOTP
	needsOTP = site.Protection == protectionOTP || site.Protection == protectionCaptchaOTP
	return needsCaptcha, needsOTP, nil
}

func GetConfig() *Config {
	if config == nil {
		config = LoadConfig()
	}
	return config
}

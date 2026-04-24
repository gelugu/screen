package configuration

import "flag"

func init() {
	flag.String("config-path", "/etc/screen/config.yaml", "path to the config file")
}

func configPath() string {
	if f := flag.Lookup("config-path"); f != nil {
		return f.Value.String()
	}
	return "/etc/screen/config.yaml"
}

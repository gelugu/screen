package logging

import (
	"screen/configuration"

	"github.com/sirupsen/logrus"
)

var globalLogger *logrus.Logger

func getGlobalLogger() *logrus.Logger {
	if globalLogger != nil {
		return globalLogger
	}

	config := configuration.GetConfig()

	logLevel, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		logrus.Fatalf("failed to parse log level %q: %v", config.LogLevel, err)
	}

	globalLogger = logrus.New()
	globalLogger.SetLevel(logLevel)
	globalLogger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	return globalLogger
}

func NewLogger(packageName string) *logrus.Entry {
	return getGlobalLogger().WithField("package", packageName)
}

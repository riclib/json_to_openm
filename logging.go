package main

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"os"
)

func SetupLog() logr.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zerologr.NameFieldName = "logger"
	zerologr.NameSeparator = "/"

	zl := zerolog.New(os.Stderr)
	if viper.GetBool("debug") {
		zl = zl.Level(zerolog.TraceLevel)
	} else {
		zl = zl.Level(zerolog.InfoLevel)

	}

	zl = zl.With().Caller().Timestamp().Logger()
	var log logr.Logger = zerologr.New(&zl)
	return log
}

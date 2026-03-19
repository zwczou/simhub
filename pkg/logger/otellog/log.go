package otellog

import (
	"github.com/go-logr/logr"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type Logger struct {
	callDepth int
	level     int
	logger    zerolog.Logger
}

var _ logr.LogSink = (*Logger)(nil)

func New(level int) *Logger {
	return &Logger{
		callDepth: 1,
		level:     level,
		logger:    log.Logger,
	}
}

func (l *Logger) Init(info logr.RuntimeInfo) {
	l.callDepth = info.CallDepth
}

func (l *Logger) Enabled(level int) bool {
	return l.level >= level
}

func (l *Logger) Info(level int, msg string, keysAndValues ...any) {
	if !l.Enabled(level) {
		return
	}
	if !viper.GetBool("log.traced") {
		return
	}
	logger := l.logger.Info()
	for i := 0; i < len(keysAndValues); i += 2 {
		logger = logger.Any(keysAndValues[i].(string), keysAndValues[i+1])
	}
	logger.Msg(msg)
}

func (l *Logger) Error(err error, msg string, keysAndValues ...any) {
	logger := l.logger.Error().Err(err)
	for i := 0; i < len(keysAndValues); i += 2 {
		logger = logger.Any(keysAndValues[i].(string), keysAndValues[i+1])
	}
	logger.Msg(msg)
}

func (l *Logger) WithValues(keysAndValues ...any) logr.LogSink {
	logger := l.logger
	for i := 0; i < len(keysAndValues); i += 2 {
		logger = logger.With().Any(keysAndValues[i].(string), keysAndValues[i+1]).Logger()
	}
	return &Logger{
		callDepth: l.callDepth,
		level:     l.level,
		logger:    logger,
	}
}

func (l *Logger) WithName(name string) logr.LogSink {
	return l.WithValues("name", name)
}

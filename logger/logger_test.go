package logger

import "testing"

func TestLoggerFromConfig(t *testing.T) {
	log := NewLogger("logger.toml")
	log.Trace("hello world")
	log.Notice("hello world")
}

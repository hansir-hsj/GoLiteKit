package logger

import (
	"fmt"
	"github/hsj/golite/config"
	"io"
	"os"
)

const (
	LevelDebug = iota
	LevelTrace
	LevelNotice
	LevelWarning
	LevelFatal
)

type Logger interface {
	Debug(msg string)
	Trace(msg string)
	Notice(msg string)
	Warning(msg string)
	Fatal(msg string)
}

type DefaultLogger struct {
	LoggerDir      string `toml:"dir"`
	LoggerFileName string `toml:"filename"`
	Level          int    `toml:"level"`

	target io.Writer
}

func NewLogger(confFile string) Logger {
	var defLogger DefaultLogger
	config.Parse(confFile, &defLogger)

	err := os.MkdirAll(defLogger.LoggerDir, 0755)
	if err != nil {
		panic(err)
	}
	target, err := os.OpenFile(fmt.Sprintf("%s/%s", defLogger.LoggerDir, defLogger.LoggerFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	defLogger.target = target

	return &defLogger
}

func (l *DefaultLogger) Debug(msg string) {
	l.Log(LevelDebug, msg)
}

func (l *DefaultLogger) Trace(msg string) {
	l.Log(LevelTrace, msg)
}

func (l *DefaultLogger) Notice(msg string) {
	l.Log(LevelNotice, msg)
}

func (l *DefaultLogger) Warning(msg string) {
	l.Log(LevelWarning, msg)
}

func (l *DefaultLogger) Fatal(msg string) {
	l.Log(LevelFatal, msg)
}

func (l *DefaultLogger) Log(level int, msg string) {
	if level < l.Level {
		return
	}
	fmt.Fprintln(l.target, msg)
}

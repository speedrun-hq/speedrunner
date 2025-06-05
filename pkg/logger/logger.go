package logger

import (
	"github.com/fatih/color"
	"log"
	"sync"
)

// Level represents the severity level of a log message.
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	NoticeLevel
	ErrorLevel
)

type Chain int

const (
	None = iota
	Eth
	Bsc
	Pol
	Arb
	Ava
	Base
	Zeta
)

var prefixes = map[Chain]string{
	None: "",
	Eth:  "[ETH]  ",
	Bsc:  "[BSC]  ",
	Pol:  "[POL]  ",
	Arb:  "[ARB]  ",
	Ava:  "[AVA]  ",
	Base: "[BASE] ",
	Zeta: "[ZETA] ",
}

var colors = map[Chain]color.Attribute{
	None: color.FgWhite,
	Eth:  color.FgHiGreen,
	Bsc:  color.FgYellow,
	Pol:  color.FgMagenta,
	Arb:  color.FgHiBlue,
	Ava:  color.FgRed,
	Base: color.FgBlue,
	Zeta: color.FgGreen,
}

// Logger is a simple interface for logging messages.
type Logger interface {
	// Info logs an informational message.
	Info(format string, args ...interface{})
	InfoWithChain(chain Chain, format string, args ...interface{})

	// Error logs an error message.
	Error(format string, args ...interface{})
	ErrorWithChain(chain Chain, format string, args ...interface{})

	// Debug logs a debug message.
	Debug(format string, args ...interface{})
	DebugWithChain(chain Chain, format string, args ...interface{})

	// Notice logs a notice message.
	Notice(format string, args ...interface{})
	NoticeWithChain(chain Chain, format string, args ...interface{})
}

// EmptyLogger is a simple implementation of the Logger interface that does nothing.
type EmptyLogger struct{}

var _ Logger = (*EmptyLogger)(nil)

func (l *EmptyLogger) Info(_ string, _ ...interface{})                        {}
func (l *EmptyLogger) InfoWithChain(_ Chain, _ string, _ ...interface{})      {}
func (l *EmptyLogger) Error(_ string, _ ...interface{})                       {}
func (l *EmptyLogger) ErrorWithChain(_ Chain, _ string, _ ...interface{})     {}
func (l *EmptyLogger) Debug(_ string, _ ...interface{})                       {}
func (l *EmptyLogger) DebugWithChain(_ Chain, _ string, _ ...interface{})     {}
func (l *EmptyLogger) Notice(_ string, _ ...interface{})                      {}
func (l *EmptyLogger) NoticeWithChain(_ Chain, _ string, args ...interface{}) {}

// StdLogger is a standard implementation of the Logger interface that logs messages to the console.
type StdLogger struct {
	enableColoring bool
	level          Level
	mu             sync.Mutex
}

var _ Logger = (*StdLogger)(nil)

func NewStdLogger(enableColoring bool, level Level) *StdLogger {
	return &StdLogger{
		enableColoring: enableColoring,
		level:          level,
	}
}

// formatMessage formats the log message with the appropriate log level, chain prefix, and coloring if enabled.
func (l *StdLogger) formatMessage(level Level, chain Chain, format string) string {
	prefix := prefixes[chain]
	if l.enableColoring {
		prefix = color.New(colors[chain]).Sprint(prefix)
	}

	var levelStr string
	switch level {
	case DebugLevel:
		levelStr = "[DEBUG]  "
	case InfoLevel:
		levelStr = "[INFO]   "
	case NoticeLevel:
		levelStr = "[NOTICE] "
	case ErrorLevel:
		levelStr = "[ERROR]  "
	}

	return prefix + levelStr + format
}

func (l *StdLogger) Info(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= InfoLevel {
		log.Printf(l.formatMessage(InfoLevel, None, format), args...)
	}
}

func (l *StdLogger) InfoWithChain(chain Chain, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= InfoLevel {
		log.Printf(l.formatMessage(InfoLevel, chain, format), args...)
	}
}

func (l *StdLogger) Error(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= ErrorLevel {
		log.Printf(l.formatMessage(ErrorLevel, None, format), args...)
	}
}

func (l *StdLogger) ErrorWithChain(chain Chain, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= ErrorLevel {
		log.Printf(l.formatMessage(ErrorLevel, chain, format), args...)
	}
}

func (l *StdLogger) Debug(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= DebugLevel {
		log.Printf(l.formatMessage(DebugLevel, None, format), args...)
	}
}

func (l *StdLogger) DebugWithChain(chain Chain, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= DebugLevel {
		log.Printf(l.formatMessage(DebugLevel, chain, format), args...)
	}
}

func (l *StdLogger) Notice(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= NoticeLevel {
		log.Printf(l.formatMessage(NoticeLevel, None, format), args...)
	}
}

func (l *StdLogger) NoticeWithChain(chain Chain, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level <= NoticeLevel {
		log.Printf(l.formatMessage(NoticeLevel, chain, format), args...)
	}
}

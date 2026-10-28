package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

type loggerKey string

const LoggerKeyName loggerKey = "log"

const (
	timeFormat  = "2006-01-02T15:04:05.000000"
	fileTimeFmt = "2006-01-02T15-04-05.000000"
	dirPerm     = 0o755
	filePerm    = 0o644
)

const stderrLevel = zerolog.WarnLevel

var ErrInvalidLogLevel = errors.New("invalid log level")

type Logger struct {
	*zerolog.Logger
	file *os.File
}

type splitWriter struct {
	out, errw io.Writer
	min       zerolog.Level
}

func (w splitWriter) Write(p []byte) (int, error) {
	return w.out.Write(p)
}

func (w splitWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level >= w.min {
		return w.errw.Write(p)
	}

	return w.out.Write(p)
}

func ValidateLogLevel(logLevel string) error {
	if logLevel == "" {
		return ErrInvalidLogLevel
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil || level == zerolog.NoLevel {
		return ErrInvalidLogLevel
	}

	return nil
}

func New(logDir, logLevel string) (*Logger, error) {
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	if err := os.MkdirAll(logDir, dirPerm); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	name := fmt.Sprintf("%s.log", time.Now().UTC().Format(fileTimeFmt))

	logFile, err := os.OpenFile(
		filepath.Join(logDir, name),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		filePerm)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano

	console := splitWriter{
		out:  zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: timeFormat},
		errw: zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: timeFormat},
		min:  stderrLevel,
	}
	fileOut := zerolog.ConsoleWriter{Out: logFile, TimeFormat: timeFormat, NoColor: true}

	zl := zerolog.New(zerolog.MultiLevelWriter(console, fileOut)).
		Level(level).
		With().
		Timestamp().
		Caller().
		Logger()

	return &Logger{
		Logger: &zl,
		file:   logFile,
	}, nil
}

func (l *Logger) With(fn func(zerolog.Context) zerolog.Context) *Logger {
	sub := fn(l.Logger.With()).Logger()

	return &Logger{
		Logger: &sub,
		file:   l.file,
	}
}

func ToContext(ctx context.Context, log *Logger) context.Context {
	return context.WithValue(ctx, LoggerKeyName, log)
}

func FromContext(ctx context.Context) *Logger {
	log, ok := ctx.Value(LoggerKeyName).(*Logger)
	if !ok {
		panic("no logger in context")
	}

	return log
}

func (l *Logger) Close() error {
	var syncErr, closeErr error

	if err := l.file.Sync(); err != nil {
		syncErr = fmt.Errorf("sync logger file: %w", err)
	}

	if err := l.file.Close(); err != nil {
		closeErr = fmt.Errorf("close logger file: %w", err)
	}

	return errors.Join(syncErr, closeErr)
}

package logging

import (
	"fmt"
	"io"
	"strings"
)

type Logger interface {
	Infof(format string, args ...any)
	Debugf(format string, args ...any)
	Writer() io.Writer
}

type WriterLogger struct {
	writer io.Writer
	debug  bool
}

func NewWriterLogger(writer io.Writer, debug bool) Logger {
	return WriterLogger{
		writer: writer,
		debug:  debug,
	}
}

func (l WriterLogger) Infof(format string, args ...any) {
	l.write(format, args...)
}

func (l WriterLogger) Debugf(format string, args ...any) {
	if !l.debug {
		return
	}
	l.write(format, args...)
}

func (l WriterLogger) Writer() io.Writer {
	return l.writer
}

func (l WriterLogger) write(format string, args ...any) {
	if l.writer == nil {
		return
	}
	_, _ = fmt.Fprintf(l.writer, format+"\n", args...)
}

func LogList(logger Logger, label string, values []string) {
	if logger == nil {
		return
	}
	if len(values) == 0 {
		logger.Infof("%s: <none>", label)
		return
	}

	const maxItems = 20
	display := values
	if len(display) > maxItems {
		display = display[:maxItems]
	}
	logger.Infof("%s (%d): %s", label, len(values), strings.Join(display, ", "))
	if len(values) > maxItems {
		logger.Infof("%s: ... %d more", label, len(values)-maxItems)
	}
}

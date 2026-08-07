package logging

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
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

type TableRow struct {
	Label string
	Value string
}

func LogTable(logger Logger, heading string, rows ...TableRow) {
	if logger == nil {
		return
	}

	var builder strings.Builder
	table := tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, heading)
	for _, row := range rows {
		_, _ = fmt.Fprintf(table, "%s\t%s\n", row.Label, row.Value)
	}
	_ = table.Flush()

	logger.Infof("%s", strings.TrimSuffix(builder.String(), "\n"))
}

func FormatList(values []string) string {
	if len(values) == 0 {
		return "<none>"
	}

	const maxItems = 20
	display := values
	if len(display) > maxItems {
		display = display[:maxItems]
	}

	formatted := fmt.Sprintf("(%d) %s", len(values), strings.Join(display, ", "))
	if len(values) > maxItems {
		formatted += fmt.Sprintf(", ... %d more", len(values)-maxItems)
	}
	return formatted
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

func LogListDebug(logger Logger, label string, values []string) {
	if logger == nil {
		return
	}
	if len(values) == 0 {
		logger.Debugf("%s: <none>", label)
		return
	}

	const maxItems = 20
	display := values
	if len(display) > maxItems {
		display = display[:maxItems]
	}
	logger.Debugf("%s (%d): %s", label, len(values), strings.Join(display, ", "))
	if len(values) > maxItems {
		logger.Debugf("%s: ... %d more", label, len(values)-maxItems)
	}
}

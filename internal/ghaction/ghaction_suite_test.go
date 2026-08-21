package ghaction

import (
	"fmt"
	"io"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGhaction(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GH action suite")
}

type testLogger struct {
	builder strings.Builder
	debug   bool
}

func newTestLogger(debug bool) *testLogger {
	return &testLogger{debug: debug}
}

func (l *testLogger) Infof(format string, args ...any) {
	_, _ = fmt.Fprintf(&l.builder, format+"\n", args...)
}

func (l *testLogger) Debugf(format string, args ...any) {
	if !l.debug {
		return
	}
	l.Infof(format, args...)
}

func (l *testLogger) Writer() io.Writer {
	return io.Discard
}

func (l *testLogger) String() string {
	return l.builder.String()
}

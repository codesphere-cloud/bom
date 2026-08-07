package cli

import (
	"io"
	"strings"
	"testing"
)

func TestRootCommandIncludesWorkflowCommands(t *testing.T) {
	cmd := New(strings.NewReader(""), io.Discard, io.Discard).RootCommand()

	for _, args := range [][]string{{"generate"}, {"check"}, {"registry", "login"}} {
		if _, _, err := cmd.Find(args); err != nil {
			t.Fatalf("find command %v: %v", args, err)
		}
	}
}

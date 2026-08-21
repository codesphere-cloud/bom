package cli

import (
	"io"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CLI suite")
}

var _ = Describe("Root command", func() {
	DescribeTable("includes workflow commands",
		func(args []string) {
			cmd := New(strings.NewReader(""), io.Discard, io.Discard).RootCommand()
			_, _, err := cmd.Find(args)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("generate", []string{"generate"}),
		Entry("check", []string{"check"}),
		Entry("registry login", []string{"registry", "login"}),
	)
})

package login

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/bom/internal/logging"
	dockerconfig "github.com/docker/cli/cli/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLogin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Login suite")
}

var _ = Describe("Run", func() {
	It("stores credentials passed directly", func() {
		dockerConfigDir := GinkgoT().TempDir()
		GinkgoT().Setenv("DOCKER_CONFIG", dockerConfigDir)

		var stdout bytes.Buffer
		err := Run(strings.NewReader(""), &stdout, logging.NewWriterLogger(io.Discard, false), Config{
			Server:   "ghcr.io",
			Username: "alice",
			Password: "secret",
		})
		Expect(err).NotTo(HaveOccurred())

		configPath := filepath.Join(dockerConfigDir, "config.json")
		_, err = os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())

		cfg, err := dockerconfig.Load(dockerConfigDir)
		Expect(err).NotTo(HaveOccurred())
		auth, err := cfg.GetAuthConfig("ghcr.io")
		Expect(err).NotTo(HaveOccurred())
		Expect(auth.Username).To(Equal("alice"))
		Expect(auth.Password).To(Equal("secret"))
	})

	It("reads the password from stdin", func() {
		dockerConfigDir := GinkgoT().TempDir()
		GinkgoT().Setenv("DOCKER_CONFIG", dockerConfigDir)

		err := Run(strings.NewReader("hunter2\n"), &bytes.Buffer{}, logging.NewWriterLogger(io.Discard, false), Config{
			Server:        "docker.io",
			Username:      "bob",
			PasswordStdin: true,
		})
		Expect(err).NotTo(HaveOccurred())

		cfg, err := dockerconfig.Load(dockerConfigDir)
		Expect(err).NotTo(HaveOccurred())
		auth, err := cfg.GetAuthConfig("docker.io")
		Expect(err).NotTo(HaveOccurred())
		Expect(auth.Username).To(Equal("bob"))
		Expect(auth.Password).To(Equal("hunter2"))
	})
})

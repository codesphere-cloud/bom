package login

import (
	"io"
	"os"

	"github.com/codesphere-cloud/bom/internal/logging"
	cranecmd "github.com/google/go-containerregistry/cmd/crane/cmd"
)

type Config struct {
	Server        string
	Username      string
	Password      string
	PasswordStdin bool
}

func Run(stdin io.Reader, stdout io.Writer, logger logging.Logger, cfg Config) error {
	restore, err := prepareCraneLoginStdin(stdin, cfg.PasswordStdin)
	if err != nil {
		return err
	}
	if restore != nil {
		defer restore()
	}

	cmd := cranecmd.NewCmdAuthLogin("bom registry")
	cmd.SetOut(stdout)
	cmd.SetErr(logger.Writer())
	cmd.SetIn(stdin)
	logger.Debugf("logging in to registry %s", cfg.Server)

	args := []string{cfg.Server, "--username", cfg.Username}
	if cfg.PasswordStdin {
		args = append(args, "--password-stdin")
	} else {
		args = append(args, "--password", cfg.Password)
	}

	cmd.SetArgs(args)
	return cmd.Execute()
}

func prepareCraneLoginStdin(stdin io.Reader, passwordStdin bool) (func(), error) {
	if !passwordStdin {
		return nil, nil
	}

	file, err := os.CreateTemp("", "bom-registry-login-*")
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(file, stdin); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}

	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}

	original := os.Stdin
	os.Stdin = file

	return func() {
		os.Stdin = original
		_ = file.Close()
		_ = os.Remove(file.Name())
	}, nil
}

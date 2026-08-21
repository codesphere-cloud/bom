package images

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestImages(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "images suite")
}

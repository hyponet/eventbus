package bus

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

func TestEventBus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fs Suite")
}

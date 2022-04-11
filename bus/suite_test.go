package bus

import (
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"testing"
)

func TestEventBus(t *testing.T) {
	config.DefaultReporterConfig.SlowSpecThreshold = 60
	RegisterFailHandler(Fail)
	RunSpecs(t, "EventBus Suite")
}

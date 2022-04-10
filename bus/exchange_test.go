package bus

import (
	"fmt"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
)

var _ = Describe("TestExchange", func() {
	var (
		ex *exchange
	)

	BeforeEach(func() {
		ex = newExchange()
	})

	Describe("", func() {
		It("test once add", func() {
			Context("test add", func() {
				ex.add("a.b.c.d", uuid.New().String())
				ex.add("a.b.c.e", uuid.New().String())
				ex.add("a.b.c.f", uuid.New().String())
				ex.add("a.b.d.c", uuid.New().String())
				ex.add("a.b.d.e", uuid.New().String())
				ex.add("a.b.d.e.f", uuid.New().String())
				fmt.Println("hello")
			})
			Context("route", func() {
			})
		})
	})
})

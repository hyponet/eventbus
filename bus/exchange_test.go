package bus

import (
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
				ex.add("a.b.c.d", "id-1")
				ex.add("a.b.c.e", "id-2")
				ex.add("a.b.c.f", "id-3")
				ex.add("a.b.d.c", "id-4")
				ex.add("a.b.d.e", "id-5")
				ex.add("a.b.d.e.f", "id-6")
			})
			Context("test delete", func() {
				ex.remove("id-1")
				ex.remove("id-2")
				ex.remove("id-3")
				ex.remove("id-4")
				ex.remove("id-5")
				ex.remove("id-5")
				ex.remove("id-6")
			})
		})
	})
})

package bus

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
				Expect(isContained(ex.route("a.b.c.d"), "id-1")).Should(BeTrue())

				ex.add("a.b.c.e", "id-2")
				Expect(isContained(ex.route("a.b.c.e"), "id-2")).Should(BeTrue())

				ex.add("a.b.c.f", "id-3")
				Expect(isContained(ex.route("a.b.c.f"), "id-3")).Should(BeTrue())

				ex.add("a.b.d.c", "id-4")
				Expect(isContained(ex.route("a.b.d.c"), "id-4")).Should(BeTrue())

				ex.add("a.b.d.e", "id-5")
				Expect(isContained(ex.route("a.b.d.e"), "id-5")).Should(BeTrue())

				ex.add("a.b.d.e.f", "id-6")
				Expect(isContained(ex.route("a.b.d.e.f"), "id-6")).Should(BeTrue())
			})
			Context("test delete", func() {
				ex.remove("id-1")
				Expect(isContained(ex.route("a.b.c.d"), "id-1")).ShouldNot(BeTrue())

				ex.remove("id-2")
				Expect(isContained(ex.route("a.b.c.e"), "id-2")).ShouldNot(BeTrue())

				ex.remove("id-3")
				Expect(isContained(ex.route("a.b.c.f"), "id-3")).ShouldNot(BeTrue())

				ex.remove("id-4")
				Expect(isContained(ex.route("a.b.d.c"), "id-4")).ShouldNot(BeTrue())

				ex.remove("id-5")
				Expect(isContained(ex.route("a.b.d.e"), "id-5")).ShouldNot(BeTrue())

				ex.remove("id-6")
				Expect(isContained(ex.route("a.b.d.e.f"), "id-6")).ShouldNot(BeTrue())
			})
		})
	})
})

var _ = Describe("TestExchangeRoute", func() {
	var (
		ex *exchange
	)

	BeforeEach(func() {
		ex = newExchange()
	})

	Describe("", func() {
		It("test wildcard route", func() {
			Context("add topics", func() {
				ex.add("a.b.c.d", "id-1")
				ex.add("a.b.c.e", "id-2")
				ex.add("a.b.c.f", "id-3")
				ex.add("a.b.d.c", "id-4")
				ex.add("a.b.d.e", "id-5")
				ex.add("a.b.d.e.f", "id-6")
			})
			Context("add topics", func() {
				Expect(isContained(ex.route("a.b.c.*"), "id-1")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.c.*"), "id-2")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.c.*"), "id-3")).Should(BeTrue())

				Expect(isContained(ex.route("a.b.*.e"), "id-2")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.*.e"), "id-5")).Should(BeTrue())

				Expect(isContained(ex.route("a.b.*.*"), "id-1")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.*.*"), "id-2")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.*.*"), "id-3")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.*.*"), "id-4")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.*.*"), "id-5")).Should(BeTrue())

				Expect(isContained(ex.route("a.b.*.*.f"), "id-6")).Should(BeTrue())
			})
		})

		It("test route wildcard", func() {
			Context("add topics", func() {
				ex.add("a.b.c.d", "id-1")
				ex.add("a.b.c.e", "id-2")
				ex.add("a.b.c.*", "id-3")
				ex.add("a.b.d.e.f", "id-4")
				ex.add("a.b.d.*.f", "id-5")
				ex.add("a.b.d.*.*", "id-6")
			})
			Context("add topics", func() {
				Expect(isContained(ex.route("a.b.c.*"), "id-1")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.c.*"), "id-2")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.c.*"), "id-3")).Should(BeTrue())

				Expect(isContained(ex.route("a.b.c.d"), "id-1")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.c.d"), "id-3")).Should(BeTrue())

				Expect(isContained(ex.route("a.b.d.e.f"), "id-4")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.d.e.f"), "id-5")).Should(BeTrue())
				Expect(isContained(ex.route("a.b.d.e.f"), "id-6")).Should(BeTrue())
			})
		})
	})
})

func isContained(values []string, val string) bool {
	for _, v := range values {
		if v == val {
			return true
		}
	}
	return false
}

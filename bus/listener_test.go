package bus

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TestListener", func() {
	var (
		l      *listener
		err    error
		topic  string
		finish chan struct{}
	)

	BeforeEach(func() {
		l = nil
		finish = make(chan struct{})
	})

	Describe("", func() {
		It("normal test", func() {
			var isExec = false
			handler := func() {
				isExec = true
				close(finish)
			}
			Context("create", func() {
				l, err = buildNewListener(topic, handler, false, false)
				Expect(err).Should(BeNil())
			})
			Context("exec", func() {
				l.call()
				<-finish
			})
			Context("check", func() {
				Expect(isExec).Should(BeTrue())
			})
		})
		It("with normal args", func() {
			var ans string
			handler := func(aNum int, aStr string) {
				ans = fmt.Sprintf("%d_%s", aNum, aStr)
				close(finish)
			}
			Context("create", func() {
				l, err = buildNewListener(topic, handler, false, false)
				Expect(err).Should(BeNil())
			})
			Context("exec", func() {
				l.call(9, "hello")
				<-finish
			})
			Context("check", func() {
				Expect(ans).Should(Equal("9_hello"))
			})
		})
		It("with struct args", func() {
			type aStruct struct {
				hello string
				next  int
			}

			var ans = aStruct{}
			handler := func(a1 aStruct, a2, a3 *aStruct) {
				ans.hello += a1.hello
				ans.next += a1.next
				if a2 != nil {
					ans.hello += a2.hello
					ans.next += a2.next
				}
				if a3 != nil {
					ans.hello += a3.hello
					ans.next += a3.next
				} else {
					ans.next += 1
				}
				close(finish)
			}

			Context("create", func() {
				l, err = buildNewListener(topic, handler, false, false)
				Expect(err).Should(BeNil())
			})
			Context("exec", func() {
				l.call(aStruct{hello: "a", next: 4}, &aStruct{hello: "b", next: 6}, nil)
				<-finish
			})
			Context("check", func() {
				Expect(ans.hello).Should(Equal("ab"))
				Expect(ans.next).Should(Equal(11))
			})
		})
	})
})

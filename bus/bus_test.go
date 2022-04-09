package bus

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sync"
	"time"
)

var _ = Describe("TestEventBusApi", func() {
	BeforeEach(func() {
		evb = &eventbus{
			listeners: map[string]*listener{},
			exchange:  newExchange(),
		}
	})
	Describe("", func() {
		It("test register handler", func() {
			Context("succeed", func() {
				Expect(Register("test.topic.a", func() {})).Should(BeNil())
				Expect(Register("test.topic.a", func(astr, bstr string) error { return nil })).Should(BeNil())
			})
			Context("not func", func() {
				Expect(Register("test.topic.c", "wrong val")).ShouldNot(BeNil())
				Expect(Register("test.topic.c", nil)).ShouldNot(BeNil())
				Expect(Register("test.topic.c", 0)).ShouldNot(BeNil())
			})
		})
		It("test publish", func() {
			var (
				isExec = false
				topic  = "test.topic.a"
			)
			Expect(Register("test.topic.a", func() {
				isExec = true
			})).Should(BeNil())

			Publish(topic)

			Eventually(func() bool {
				return isExec == true
			}, time.Minute, time.Second).Should(BeTrue())
		})
		It("test unregister handler", func() {
			var (
				isExec = false
				topic  = "test.topic.a"
			)
			Expect(Register("test.topic.a", func() {
				isExec = true
			})).Should(BeNil())
			Publish(topic)
			time.Sleep(time.Second * 5)
			Expect(isExec).Should(BeFalse())
		})
	})
})

var _ = Describe("TestEventBus", func() {
	var (
		testBus        *eventbus
		l              *listener
		topic          = "a.b.c.d"
		runTimes       int
		unsafeRunTimes int
		mux            sync.Mutex
	)

	runFn := func() {
		unsafeRunTimes += 1
		mux.Lock()
		runTimes += 1
		mux.Unlock()
	}
	BeforeEach(func() {
		testBus = &eventbus{
			listeners: map[string]*listener{},
			exchange:  newExchange(),
		}

		runTimes = 0
		unsafeRunTimes = 0
	})

	Describe("", func() {
		It("test normal func", func() {
			Context("run many", func() {
				var err error
				l, err = buildNewListener(topic, runFn, false, false)
				Expect(err).Should(BeNil())
				testBus.register(l)

				needRun := 1000
				for i := 0; i < needRun; i++ {
					testBus.publish(topic)
				}
				Eventually(func() bool {
					return runTimes == needRun
				}, time.Minute, time.Second).Should(BeTrue())
			})
		})
		It("test block func", func() {
			Context("run many", func() {
				var err error
				l, err = buildNewListener(topic, runFn, true, false)
				Expect(err).Should(BeNil())
				testBus.register(l)

				needRun := 1000
				for i := 0; i < needRun; i++ {
					testBus.publish(topic)
				}
				Eventually(func() bool {
					return runTimes == needRun
				}, time.Minute, time.Second).Should(BeTrue())
				Expect(unsafeRunTimes).Should(Equal(needRun))
			})
		})
		It("test once func", func() {
			Context("run once", func() {
				var err error
				l, err = buildNewListener(topic, runFn, false, true)
				Expect(err).Should(BeNil())
				testBus.register(l)

				needRun := 10
				for i := 0; i < needRun; i++ {
					testBus.publish(topic)
				}
				time.Sleep(time.Second * 5)
				Expect(unsafeRunTimes).Should(Equal(1))
			})
		})
	})
})

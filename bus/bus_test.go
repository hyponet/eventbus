package bus

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sync"
	"time"
)

var _ = Describe("TestEventBusApi", func() {
	BeforeEach(func() {
		sb = &Bus{
			listeners: map[string]*Listener{},
			exchange:  newExchange(),
		}
	})
	Describe("", func() {
		It("test subscribe handler", func() {
			Context("succeed", func() {
				Subscribe("test.topic.a", func() {})
				Subscribe("test.topic.a", func(astr, bstr string) error { return nil })
			})
		})
		It("test publish", func() {
			var (
				isExec = false
				topic  = "test.topic.a"
			)
			Subscribe("test.topic.a", func() { isExec = true })
			Publish(topic)
			Eventually(func() bool {
				return isExec == true
			}, time.Minute, time.Second).Should(BeTrue())
		})
		It("test unsubscribe handler", func() {
			var (
				lID    string
				isExec = false
				topic  = "test.topic.a"
			)
			lID = Subscribe("test.topic.a", func() { isExec = true })
			Unsubscribe(lID)
			Publish(topic)
			time.Sleep(time.Second * 5)
			Expect(isExec).Should(BeFalse())
		})
	})
})

var _ = Describe("TestEventBus", func() {
	var (
		testBus        *Bus
		l              *Listener
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
		testBus = &Bus{
			listeners: map[string]*Listener{},
			exchange:  newExchange(),
		}

		runTimes = 0
		unsafeRunTimes = 0
	})

	Describe("", func() {
		It("test normal func", func() {
			Context("run many", func() {
				l = NewListener(topic, runFn, false, false)
				testBus.Subscribe(l)

				needRun := 1000
				for i := 0; i < needRun; i++ {
					testBus.Publish(topic)
				}
				Eventually(func() bool {
					return runTimes == needRun
				}, time.Minute, time.Second).Should(BeTrue())
			})
		})
		It("test block func", func() {
			Context("run many", func() {
				l = NewListener(topic, runFn, true, false)
				testBus.Subscribe(l)

				needRun := 1000
				for i := 0; i < needRun; i++ {
					testBus.Publish(topic)
				}
				Eventually(func() bool {
					return runTimes == needRun
				}, time.Minute, time.Second).Should(BeTrue())
				Expect(unsafeRunTimes).Should(Equal(needRun))
			})
		})
		It("test once func", func() {
			Context("run once", func() {
				l = NewListener(topic, runFn, false, true)
				testBus.Subscribe(l)

				needRun := 10
				for i := 0; i < needRun; i++ {
					testBus.Publish(topic)
				}
				time.Sleep(time.Second * 5)
				Expect(runTimes).Should(Equal(1))
			})
		})
	})
})

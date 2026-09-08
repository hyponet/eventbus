package bus

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sync"
	"sync/atomic"
	"time"
)

var _ = Describe("TestEventBusApi", func() {
	BeforeEach(func() {
		sb = NewBus()
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
				isExec atomic.Bool
				topic  = "test.topic.a"
			)
			Subscribe("test.topic.a", func() { isExec.Store(true) })
			Publish(topic)
			Eventually(func() bool {
				return isExec.Load() == true
			}, time.Minute, time.Second).Should(BeTrue())
		})
		It("test unsubscribe handler", func() {
			var (
				lID    string
				isExec atomic.Bool
				topic  = "test.topic.a"
			)
			lID = Subscribe("test.topic.a", func() { isExec.Store(true) })
			Unsubscribe(lID)
			Publish(topic)
			time.Sleep(time.Second * 5)
			Expect(isExec.Load()).Should(BeFalse())
		})
	})
})

var _ = Describe("TestEventBus", func() {
	var (
		testBus  *Bus
		l        *Listener
		topic    = "a.b.c.d"
		runTimes int
		// atomic so the unsynchronized invocation counter itself does
		// not trip the race detector; ordering guarantees are asserted
		// via runTimes (mutex-guarded) and the serial tests.
		unsafeRunTimes atomic.Int64
		mux            sync.Mutex
	)

	countRunTimes := func() int {
		mux.Lock()
		defer mux.Unlock()
		return runTimes
	}
	runFn := func() {
		unsafeRunTimes.Add(1)
		mux.Lock()
		runTimes += 1
		mux.Unlock()
	}
	BeforeEach(func() {
		testBus = NewBus()

		runTimes = 0
		unsafeRunTimes.Store(0)
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
					return countRunTimes() == needRun
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
					return countRunTimes() == needRun
				}, time.Minute, time.Second).Should(BeTrue())
				Expect(unsafeRunTimes.Load()).Should(Equal(int64(needRun)))
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
				Expect(countRunTimes()).Should(Equal(1))
			})
		})
	})
})

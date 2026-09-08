package bus

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cleanupBus(t *testing.T, b *Bus) {
	t.Helper()
	t.Cleanup(func() {
		b.Close()
		b.Wait()
	})
}

// TestSubscribeSerialOrder verifies that every publisher's messages retain
// their order when concurrent publishers share one serial listener.
func TestSubscribeSerialOrder(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	const (
		publishers = 8
		perPub     = 1000
	)
	var (
		mu     sync.Mutex
		last   = make([]int, publishers)
		outOfO bool
	)
	lid := b.SubscribeSerial("order.topic", func(p, seq int) {
		mu.Lock()
		if seq <= last[p] {
			outOfO = true
		}
		last[p] = seq
		mu.Unlock()
	}, WithBlock())

	start := make(chan struct{})
	var wg sync.WaitGroup
	for p := 0; p < publishers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			<-start
			for seq := 1; seq <= perPub; seq++ {
				b.Publish("order.topic", p, seq)
			}
		}(p)
	}
	close(start)
	wg.Wait()
	b.Unsubscribe(lid)
	b.Wait()

	mu.Lock()
	defer mu.Unlock()
	if outOfO {
		t.Fatal("out-of-order delivery detected for a serial listener")
	}
	for publisher, seq := range last {
		if seq != perPub {
			t.Fatalf("publisher %d ended at sequence %d, want %d", publisher, seq, perPub)
		}
	}
}

func TestWildcardSingleSection(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var gotReplyWildcard, gotTopWildcard atomic.Int32
	l1 := b.SubscribeSerial("agent.s1.reply.*", func() { gotReplyWildcard.Add(1) })
	l2 := b.SubscribeSerial("agent.s1.*", func() { gotTopWildcard.Add(1) })

	b.Publish("agent.s1.reply.content")
	b.Publish("agent.s1.reply.content.x")
	b.Publish("agent.s1.tool_use.bash")
	b.Publish("agent.s1.inbox")
	b.Unsubscribe(l1)
	b.Unsubscribe(l2)
	b.Wait()

	if gotReplyWildcard.Load() != 1 {
		t.Fatalf("reply wildcard matched %d times, want 1", gotReplyWildcard.Load())
	}
	if gotTopWildcard.Load() != 1 {
		t.Fatalf("top wildcard matched %d times, want 1", gotTopWildcard.Load())
	}
}

func TestSerialOverflowDropOldest(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var (
		mu             sync.Mutex
		got            []int
		dropped        atomic.Uint64
		handlerStarted = make(chan struct{})
		releaseHandler = make(chan struct{})
		releaseOnce    sync.Once
	)
	release := func() { releaseOnce.Do(func() { close(releaseHandler) }) }
	t.Cleanup(release)

	lid := b.SubscribeSerial("drop.topic", func(v int) {
		if v == 1 {
			close(handlerStarted)
			<-releaseHandler
		}
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	}, WithBuffer(4), WithDropCallback(func(_ string, n uint64) {
		dropped.Store(n)
	}))

	b.Publish("drop.topic", 1)
	<-handlerStarted
	for i := 2; i <= 10; i++ {
		b.Publish("drop.topic", i)
	}
	release()
	b.Unsubscribe(lid)
	b.Wait()

	mu.Lock()
	defer mu.Unlock()
	want := []int{1, 7, 8, 9, 10}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if dropped.Load() != 5 {
		t.Fatalf("dropped = %d, want 5", dropped.Load())
	}
}

func TestSerialOverflowBlock(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var (
		mu             sync.Mutex
		got            []int
		handlerStarted = make(chan struct{})
		releaseHandler = make(chan struct{})
		releaseOnce    sync.Once
	)
	release := func() { releaseOnce.Do(func() { close(releaseHandler) }) }
	t.Cleanup(release)

	lid := b.SubscribeSerial("block.topic", func(v int) {
		if v == 1 {
			close(handlerStarted)
			<-releaseHandler
		}
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	}, WithBuffer(1), WithBlock(), WithDropCallback(func(string, uint64) {
		t.Error("WithBlock listener dropped a message")
	}))

	b.Publish("block.topic", 1)
	<-handlerStarted
	b.Publish("block.topic", 2)
	publishDone := make(chan struct{})
	go func() {
		b.Publish("block.topic", 3)
		close(publishDone)
	}()
	select {
	case <-publishDone:
		t.Fatal("publisher did not block while the queue was full")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case <-publishDone:
	case <-time.After(time.Second):
		t.Fatal("publisher remained blocked after capacity became available")
	}
	b.Unsubscribe(lid)
	b.Wait()

	mu.Lock()
	defer mu.Unlock()
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestUnsubscribeDuringPublish(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var handled atomic.Int64
	lid := b.SubscribeSerial("race.topic", func(int) { handled.Add(1) })
	stop := make(chan struct{})
	publishDone := make(chan struct{})
	go func() {
		defer close(publishDone)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				b.Publish("race.topic", i)
			}
		}
	}()

	time.Sleep(20 * time.Millisecond)
	b.Unsubscribe(lid)
	b.Wait()
	after := handled.Load()
	close(stop)
	<-publishDone
	time.Sleep(20 * time.Millisecond)
	if handled.Load() != after {
		t.Fatalf("handler ran after Wait returned: %d -> %d", after, handled.Load())
	}
}

func TestNewBusIsolation(t *testing.T) {
	b1, b2 := NewBus(), NewBus()
	cleanupBus(t, b1)
	cleanupBus(t, b2)

	var got atomic.Int32
	lid := b1.SubscribeSerial("iso.topic", func(v int) { got.Add(int32(v)) })
	b2.Publish("iso.topic", 1)
	time.Sleep(20 * time.Millisecond)
	if got.Load() != 0 {
		t.Fatal("bus instance received a message published to another instance")
	}
	b1.Publish("iso.topic", 1)
	b1.Unsubscribe(lid)
	b1.Wait()
	if got.Load() != 1 {
		t.Fatal("bus instance did not receive its own message")
	}
}

func TestBusCloseAndWait(t *testing.T) {
	runtime.GC()
	before := runtime.NumGoroutine()
	b := NewBus()
	for i := 0; i < 10; i++ {
		b.SubscribeSerial("close.topic", func(int) {})
	}
	b.Publish("close.topic", 1)
	b.Close()
	b.Wait()
	b.Publish("close.topic", 2)

	runtime.GC()
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Fatalf("goroutines leaked by Close/Wait: before=%d after=%d", before, after)
	}
}

func TestSerialHandlerCanUnsubscribeItself(t *testing.T) {
	b := NewBus()
	done := make(chan struct{})
	var lid string
	lid = b.SubscribeSerial("self.unsubscribe", func() {
		b.Unsubscribe(lid)
		close(done)
	})
	b.Publish("self.unsubscribe")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("serial handler deadlocked while unsubscribing itself")
	}
	b.Wait()
}

func TestSerialHandlerCanCloseBus(t *testing.T) {
	b := NewBus()
	done := make(chan struct{})
	b.SubscribeSerial("self.close", func() {
		b.Close()
		close(done)
	})
	b.Publish("self.close")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("serial handler deadlocked while closing its bus")
	}
	b.Wait()
}

func TestDropCallbackCanReenterBus(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseHandler) }) }
	t.Cleanup(release)
	var reentered atomic.Bool
	lid := b.SubscribeSerial("drop.reentry", func(int) {
		select {
		case <-handlerStarted:
		default:
			close(handlerStarted)
		}
		<-releaseHandler
	}, WithBuffer(1), WithDropCallback(func(string, uint64) {
		if reentered.CompareAndSwap(false, true) {
			b.Publish("drop.reentry", 99)
		}
	}))

	b.Publish("drop.reentry", 1)
	<-handlerStarted
	b.Publish("drop.reentry", 2)
	publishDone := make(chan struct{})
	go func() {
		b.Publish("drop.reentry", 3)
		close(publishDone)
	}()
	select {
	case <-publishDone:
	case <-time.After(time.Second):
		t.Fatal("drop callback deadlocked while re-entering the bus")
	}
	release()
	b.Unsubscribe(lid)
	b.Wait()
}

func TestWaitTracksConcurrentlyClosingListener(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseHandler) }) }
	t.Cleanup(release)
	lid := b.SubscribeSerial("concurrent.close", func() {
		close(handlerStarted)
		<-releaseHandler
	})
	b.Publish("concurrent.close")
	<-handlerStarted

	b.Unsubscribe(lid)
	b.Close()
	waitDone := make(chan struct{})
	go func() {
		b.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		t.Fatal("Wait returned before the in-flight handler completed")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after the handler completed")
	}
}

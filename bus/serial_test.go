package bus

import (
	"fmt"
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
	lid := b.SubscribeSerial([]string{"order.topic"}, func(p, seq int) {
		mu.Lock()
		if seq <= last[p] {
			outOfO = true
		}
		last[p] = seq
		mu.Unlock()
	}, SerialConfig{Overflow: OverflowBlock})

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
	l1 := b.SubscribeSerial([]string{"agent.s1.reply.*"}, func() { gotReplyWildcard.Add(1) }, SerialConfig{Overflow: OverflowBlock})
	l2 := b.SubscribeSerial([]string{"agent.s1.*"}, func() { gotTopWildcard.Add(1) }, SerialConfig{Overflow: OverflowBlock})

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

	lid := b.SubscribeSerial([]string{"drop.topic"}, func(v int) {
		if v == 1 {
			close(handlerStarted)
			<-releaseHandler
		}
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	}, SerialConfig{
		Buffer:   4,
		Overflow: OverflowDropOldest,
		OnDrop:   func(delta uint64) { dropped.Add(delta) },
	})

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

	lid := b.SubscribeSerial([]string{"block.topic"}, func(v int) {
		if v == 1 {
			close(handlerStarted)
			<-releaseHandler
		}
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	}, SerialConfig{Buffer: 1, Overflow: OverflowBlock})

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
	lid := b.SubscribeSerial([]string{"race.topic"}, func(int) { handled.Add(1) }, SerialConfig{Overflow: OverflowDropOldest})
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
	lid := b1.SubscribeSerial([]string{"iso.topic"}, func(v int) { got.Add(int32(v)) }, SerialConfig{Overflow: OverflowBlock})
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
		b.SubscribeSerial([]string{"close.topic"}, func(int) {}, SerialConfig{Overflow: OverflowBlock})
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
	lid = b.SubscribeSerial([]string{"self.unsubscribe"}, func() {
		b.Unsubscribe(lid)
		close(done)
	}, SerialConfig{Overflow: OverflowBlock})
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
	b.SubscribeSerial([]string{"self.close"}, func() {
		b.Close()
		close(done)
	}, SerialConfig{Overflow: OverflowBlock})
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
	lid := b.SubscribeSerial([]string{"drop.reentry"}, func(int) {
		select {
		case <-handlerStarted:
		default:
			close(handlerStarted)
		}
		<-releaseHandler
	}, SerialConfig{Buffer: 1, Overflow: OverflowDropOldest, OnDrop: func(uint64) {
		if reentered.CompareAndSwap(false, true) {
			b.Publish("drop.reentry", 99)
		}
	}})

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
	lid := b.SubscribeSerial([]string{"concurrent.close"}, func() {
		close(handlerStarted)
		<-releaseHandler
	}, SerialConfig{Overflow: OverflowBlock})
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

// TestSubscribeSerialCrossTopicOrder verifies the core cross-topic
// guarantee: one listener subscribed to several topics receives handler
// calls in exact publish order, even when the publishes interleave
// topics. This is what streaming consumers (deltas on one topic,
// lifecycle markers on others) rely on.
func TestSubscribeSerialCrossTopicOrder(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	topics := []string{"a.run.started", "a.reply.content", "a.tool_call.bash", "a.run.finished"}
	var (
		mu  sync.Mutex
		got []string
	)
	lid := b.SubscribeSerial(topics, func(topic, seq string) {
		mu.Lock()
		got = append(got, seq)
		mu.Unlock()
	}, SerialConfig{Overflow: OverflowBlock})

	const rounds = 500
	for i := 0; i < rounds; i++ {
		for _, tp := range topics {
			b.Publish(tp, tp, fmt.Sprintf("%s#%d", tp, i))
		}
	}
	b.Unsubscribe(lid)
	b.Wait()

	mu.Lock()
	defer mu.Unlock()
	wantLen := rounds * len(topics)
	if len(got) != wantLen {
		t.Fatalf("received %d messages, want %d", len(got), wantLen)
	}
	for i := 0; i < rounds; i++ {
		for j, tp := range topics {
			want := fmt.Sprintf("%s#%d", tp, i)
			if got[i*len(topics)+j] != want {
				t.Fatalf("cross-topic order broken at %d: got %q, want %q", i*len(topics)+j, got[i*len(topics)+j], want)
			}
		}
	}
}

// TestSubscribeSerialCrossTopicConcurrentPublishers keeps the per-publisher
// cross-topic ordering guarantee while publishers compete: each publisher's
// messages stay ordered even though they cycle through topics.
func TestSubscribeSerialCrossTopicConcurrentPublishers(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	topics := []string{"x.run.*", "x.reply.content", "x.tool_call.*", "x.obs.step"}
	var (
		mu          sync.Mutex
		last        = make([]int, 8)
		outOfO      bool
		active      atomic.Int32
		overlapping atomic.Bool
	)
	lid := b.SubscribeSerial(topics, func(p, seq int) {
		if active.Add(1) != 1 {
			overlapping.Store(true)
		}
		defer active.Add(-1)
		runtime.Gosched()
		mu.Lock()
		if seq <= last[p] {
			outOfO = true
		}
		last[p] = seq
		mu.Unlock()
	}, SerialConfig{Overflow: OverflowBlock})

	start := make(chan struct{})
	var wg sync.WaitGroup
	for p := 0; p < 8; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			<-start
			for seq := 1; seq <= 1000; seq++ {
				b.Publish(topics[seq%len(topics)], p, seq)
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
		t.Fatal("out-of-order delivery detected across topics")
	}
	if overlapping.Load() {
		t.Fatal("serial handler calls overlapped")
	}
}

// TestSubscribeSerialOverlappingPatterns verifies that a listener
// registered on overlapping patterns ("run.*" and "run.finished")
// receives each publish exactly once.
func TestSubscribeSerialOverlappingPatterns(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var got atomic.Int32
	lid := b.SubscribeSerial([]string{"a.run.*", "a.run.finished", "a.*"}, func() {
		got.Add(1)
	}, SerialConfig{Overflow: OverflowBlock})
	b.Publish("a.run.finished")
	b.Publish("a.run.started")
	b.Unsubscribe(lid)
	b.Wait()

	if got.Load() != 2 {
		t.Fatalf("handler called %d times for 2 publishes, want 2", got.Load())
	}
}

// TestUnsubscribeRemovesAllTopics verifies Unsubscribe fully detaches a
// multi-topic listener: no pattern keeps delivering afterwards.
func TestUnsubscribeRemovesAllTopics(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var got atomic.Int32
	lid := b.SubscribeSerial([]string{"u.reply.content", "u.tool_call.*"}, func() {
		got.Add(1)
	}, SerialConfig{Overflow: OverflowBlock})
	b.Publish("u.reply.content")
	b.Unsubscribe(lid)
	b.Wait()

	before := got.Load()
	b.Publish("u.reply.content")
	b.Publish("u.tool_call.bash")
	if got.Load() != before {
		t.Fatalf("handler ran after multi-topic Unsubscribe: %d -> %d", before, got.Load())
	}
}

// TestDropCallbackReportsDelta verifies every drop notification reports the
// number of messages discarded by that notification.
func TestDropCallbackReportsDelta(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseHandler) }) }
	t.Cleanup(release)

	var (
		dropped      atomic.Uint64
		invalidDelta atomic.Bool
	)
	lid := b.SubscribeSerial([]string{"d.reply.content", "d.run.*"}, func(v int) {
		if v == 1 {
			close(handlerStarted)
			<-releaseHandler
		}
	}, SerialConfig{Buffer: 2, Overflow: OverflowDropOldest, OnDrop: func(delta uint64) {
		if delta != 1 {
			invalidDelta.Store(true)
		}
		dropped.Add(delta)
	}})

	b.Publish("d.reply.content", 1)
	<-handlerStarted
	b.Publish("d.run.finished", 2) // queued
	b.Publish("d.reply.content", 3)
	b.Publish("d.run.started", 4)   // overflows, drops oldest
	b.Publish("d.reply.content", 5) // overflows again
	release()
	b.Unsubscribe(lid)
	b.Wait()

	if got := dropped.Load(); got != 2 {
		t.Fatalf("drop callback count = %d, want 2", got)
	}
	if invalidDelta.Load() {
		t.Fatal("drop callback reported a non-unit delta")
	}
}

func TestDropCallbackDeltaWithConcurrentPublishers(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var (
		dropped      atomic.Uint64
		invalidDelta atomic.Bool
	)
	lid := b.SubscribeSerial([]string{"drop.concurrent"}, func(int) {
		select {
		case <-handlerStarted:
		default:
			close(handlerStarted)
		}
		<-releaseHandler
	}, SerialConfig{Buffer: 1, Overflow: OverflowDropOldest, OnDrop: func(delta uint64) {
		if delta != 1 {
			invalidDelta.Store(true)
		}
		dropped.Add(delta)
	}})

	b.Publish("drop.concurrent", -1)
	<-handlerStarted
	b.Publish("drop.concurrent", 0) // keep the queue full while publishers compete

	const publishers = 100
	var wg sync.WaitGroup
	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			b.Publish("drop.concurrent", v)
		}(i)
	}
	wg.Wait()
	close(releaseHandler)
	b.Unsubscribe(lid)
	b.Wait()

	if got := dropped.Load(); got != publishers {
		t.Fatalf("drop callback total = %d, want %d", got, publishers)
	}
	if invalidDelta.Load() {
		t.Fatal("drop callback reported a non-unit delta")
	}
}

// TestWildcardSiblingDoesNotBlockExactTopic verifies the radix lookup
// scans the whole sibling chain: a wildcard node that only partially
// matches (e.g. "agent.*.inbox" vs "agent.s1.reply.content") must not
// stop traversal before an exact-match sibling later in the chain.
func TestWildcardSiblingDoesNotBlockExactTopic(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var gotContent, gotInbox atomic.Int32
	inboxLid := b.SubscribeSerial([]string{"agent.*.inbox"}, func() { gotInbox.Add(1) }, SerialConfig{Overflow: OverflowBlock})
	lid := b.SubscribeSerial([]string{
		"agent.s1.run.*",
		"agent.s1.reply.content",
		"agent.s1.reply.reasoning",
	}, func() { gotContent.Add(1) }, SerialConfig{Overflow: OverflowBlock})

	b.Publish("agent.s1.inbox")
	b.Publish("agent.s1.reply.content")
	b.Publish("agent.s1.reply.reasoning")
	b.Publish("agent.s1.run.finished")
	b.Unsubscribe(lid)
	b.Unsubscribe(inboxLid)
	b.Wait()

	if gotInbox.Load() != 1 {
		t.Fatalf("inbox wildcard matched %d times, want 1", gotInbox.Load())
	}
	if gotContent.Load() != 3 {
		t.Fatalf("exact topics matched %d times, want 3", gotContent.Load())
	}
}

func TestSubscribeSerialDeduplicatesTopics(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	var got atomic.Int32
	lid := b.SubscribeSerial([]string{"duplicate.topic", "duplicate.topic"}, func() {
		got.Add(1)
	}, SerialConfig{Overflow: OverflowBlock})
	b.mux.RLock()
	registeredTopics := len(b.exchange.topicMapping[lid])
	b.mux.RUnlock()
	b.Publish("duplicate.topic")
	b.Unsubscribe(lid)
	b.Wait()

	if got.Load() != 1 {
		t.Fatalf("handler called %d times, want 1", got.Load())
	}
	if registeredTopics != 1 {
		t.Fatalf("registered %d duplicate topics, want 1", registeredTopics)
	}
}

func TestSerialRingBufferWraparound(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	started := make(chan struct{})
	releaseHandler := make(chan struct{})
	var got []int
	lid := b.SubscribeSerial([]string{"ring.topic"}, func(v int) {
		if v == 1 {
			close(started)
			<-releaseHandler
		}
		got = append(got, v)
	}, SerialConfig{Buffer: 3, Overflow: OverflowBlock})

	b.Publish("ring.topic", 1)
	<-started
	b.Publish("ring.topic", 2)
	b.Publish("ring.topic", 3)
	b.Publish("ring.topic", 4) // wraps to the slot consumed by message 1
	close(releaseHandler)
	b.Unsubscribe(lid)
	b.Wait()

	want := []int{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSerialPublishSnapshotsArgumentSlice(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	started := make(chan struct{})
	releaseHandler := make(chan struct{})
	var got []int
	lid := b.SubscribeSerial([]string{"snapshot.topic"}, func(v int) {
		if v == 1 {
			close(started)
			<-releaseHandler
		}
		got = append(got, v)
	}, SerialConfig{Buffer: 2, Overflow: OverflowBlock})

	b.Publish("snapshot.topic", 1)
	<-started
	args := []interface{}{2}
	b.Publish("snapshot.topic", args...)
	args[0] = 99
	close(releaseHandler)
	b.Unsubscribe(lid)
	b.Wait()

	want := []int{1, 2}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSubscribeSerialConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		topics []string
		config SerialConfig
	}{
		{name: "empty topics", config: SerialConfig{Overflow: OverflowBlock}},
		{name: "negative buffer", topics: []string{"config.topic"}, config: SerialConfig{Buffer: -1, Overflow: OverflowBlock}},
		{name: "missing overflow policy", topics: []string{"config.topic"}},
		{name: "unknown overflow policy", topics: []string{"config.topic"}, config: SerialConfig{Overflow: OverflowPolicy(255)}},
		{name: "drop callback with block", topics: []string{"config.topic"}, config: SerialConfig{Overflow: OverflowBlock, OnDrop: func(uint64) {}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBus()
			cleanupBus(t, b)
			defer func() {
				if recover() == nil {
					t.Fatal("SubscribeSerial did not panic")
				}
			}()
			b.SubscribeSerial(tt.topics, func() {}, tt.config)
		})
	}
}

func TestSubscribeSerialDefaultBuffer(t *testing.T) {
	b := NewBus()
	cleanupBus(t, b)

	lid := b.SubscribeSerial([]string{"default.buffer"}, func() {}, SerialConfig{Overflow: OverflowBlock})
	b.mux.RLock()
	capacity := b.listeners[lid].capacity
	b.mux.RUnlock()
	b.Unsubscribe(lid)
	b.Wait()

	if capacity != defaultSerialBuffer {
		t.Fatalf("capacity = %d, want %d", capacity, defaultSerialBuffer)
	}
}

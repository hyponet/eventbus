package bus

import "sync"

var (
	sb *Bus
)

func init() {
	sb = NewBus()
}

type Bus struct {
	listeners map[string]*Listener
	serials   map[string]*Listener
	exchange  *exchange
	mux       sync.RWMutex
}

// NewBus creates an isolated Bus instance. The package-level functions
// (Subscribe, Publish, ...) operate on a process-global default Bus;
// NewBus is for callers that need instance isolation (tests, embedded
// runtimes).
func NewBus() *Bus {
	return &Bus{
		listeners: map[string]*Listener{},
		serials:   map[string]*Listener{},
		exchange:  newExchange(),
	}
}

func (b *Bus) Subscribe(l *Listener) {
	b.mux.Lock()
	b.subscribeWithLock(l, []string{l.topic})
	b.mux.Unlock()
}

func (b *Bus) subscribeWithLock(l *Listener, topics []string) {
	b.listeners[l.id] = l
	for _, topic := range topics {
		b.exchange.add(topic, l.id)
	}
	if l.serial {
		b.serials[l.id] = l
	}
}

// Unsubscribe stops new messages from being delivered to lid. For a serial
// listener, messages already queued are drained asynchronously. Call Wait
// after Unsubscribe when the caller must observe completed delivery.
func (b *Bus) Unsubscribe(lid string) {
	b.mux.Lock()
	l, ok := b.listeners[lid]
	if ok {
		b.unsubscribeWithLock(lid)
	} else {
		// Keep shutdown idempotent for concurrent Unsubscribe and Close calls.
		l, ok = b.serials[lid]
	}
	b.mux.Unlock()
	if ok && l.serial {
		l.shutdown()
	}
}

func (b *Bus) unsubscribeWithLock(lid string) {
	delete(b.listeners, lid)
	b.exchange.remove(lid)
}

func (b *Bus) Publish(topic string, args ...interface{}) {
	var needDo []*Listener
	b.mux.Lock()
	lIDs := b.exchange.route(topic)
	for i, lID := range lIDs {
		needDo = append(needDo, b.listeners[lID])
		if needDo[i].once {
			b.unsubscribeWithLock(lID)
		}
	}
	b.mux.Unlock()

	for i := range needDo {
		l := needDo[i]
		if l.serial {
			l.deliver(args)
			continue
		}
		go func() {
			l.call(args...)
		}()
	}
}

// Close unsubscribes every listener on the bus and starts an asynchronous
// drain of every serial listener. Publishing after Close is a no-op until a
// new subscription is added; the bus remains reusable. Call Wait to wait for
// all serial listeners that existed when Wait began.
func (b *Bus) Close() {
	b.mux.Lock()
	serials := make([]*Listener, 0, len(b.serials))
	for _, l := range b.listeners {
		b.unsubscribeWithLock(l.id)
	}
	for _, l := range b.serials {
		serials = append(serials, l)
	}
	b.mux.Unlock()
	for _, l := range serials {
		l.shutdown()
	}
}

// Wait waits for all serial listeners that existed when Wait began to finish
// draining. Call Close or Unsubscribe first. Wait must not be called from one
// of those listeners' handlers.
func (b *Bus) Wait() {
	b.mux.RLock()
	done := make([]<-chan struct{}, 0, len(b.serials))
	for _, l := range b.serials {
		done = append(done, l.done)
	}
	b.mux.RUnlock()
	for _, ch := range done {
		<-ch
	}
}

// OverflowPolicy controls what a serial listener does when its queue is full.
// A policy must be explicitly selected in SerialConfig.
type OverflowPolicy uint8

const (
	// OverflowBlock blocks Publish until the listener has queue capacity.
	OverflowBlock OverflowPolicy = iota + 1
	// OverflowDropOldest discards the oldest queued message before enqueueing
	// the new message.
	OverflowDropOldest
)

// SerialConfig configures the bounded FIFO owned by a serial listener.
type SerialConfig struct {
	// Buffer is the number of messages that may wait behind the in-flight
	// handler call. Zero uses the default of 256; negative values are invalid.
	Buffer int
	// Overflow is required and determines what happens when Buffer is full.
	Overflow OverflowPolicy
	// OnDrop is called after each OverflowDropOldest discard with the number
	// of messages discarded by that notification (currently always one).
	// Concurrent publishers may invoke it concurrently. It must be nil with
	// OverflowBlock.
	OnDrop func(delta uint64)
}

// SubscribeSerial registers fn on every pattern in topics as ONE serial
// listener: a single bounded FIFO queue drained by a single dedicated
// goroutine. Handler calls happen exactly in the order the publishes
// entered the queue, so ordering holds not only within one topic but
// ACROSS all subscribed topics. This is the default and only serial
// subscription model — consumers that merge several topics get source
// order for free, provided a single publisher goroutine publishes the
// related stream (per-publisher order is preserved when publishers
// compete; their interleaving is inherently arbitrary).
//
// Patterns may overlap (e.g. "run.*" and "run.finished"): a publish that
// matches several of the listener's patterns is still delivered exactly
// once. Queue overflow behavior must be selected explicitly in config.
//
// SubscribeSerial does not support one-shot semantics. It panics when
// topics is empty or config is invalid. The handler receives exactly the
// arguments passed to Publish; the matching topic is not injected.
func (b *Bus) SubscribeSerial(topics []string, fn interface{}, config SerialConfig) string {
	if len(topics) == 0 {
		panic("SubscribeSerial requires at least one topic")
	}
	if config.Buffer < 0 {
		panic("SubscribeSerial buffer must not be negative")
	}
	if config.Buffer == 0 {
		config.Buffer = defaultSerialBuffer
	}
	switch config.Overflow {
	case OverflowBlock:
		if config.OnDrop != nil {
			panic("SubscribeSerial OnDrop must be nil with OverflowBlock")
		}
	case OverflowDropOldest:
	default:
		panic("SubscribeSerial requires a valid overflow policy")
	}

	uniqueTopics := make([]string, 0, len(topics))
	seenTopics := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, exists := seenTopics[topic]; exists {
			continue
		}
		seenTopics[topic] = struct{}{}
		uniqueTopics = append(uniqueTopics, topic)
	}

	l := NewListener(uniqueTopics[0], fn, false, false)
	l.serial = true
	l.capacity = config.Buffer
	l.queue = make([][]interface{}, config.Buffer)
	l.overflow = config.Overflow
	l.onDrop = config.OnDrop
	l.done = make(chan struct{})
	l.cond = sync.NewCond(&l.mux)

	b.mux.Lock()
	b.subscribeWithLock(l, uniqueTopics)
	b.mux.Unlock()

	go func() {
		defer func() {
			b.mux.Lock()
			delete(b.serials, l.id)
			b.mux.Unlock()
			close(l.done)
		}()
		l.run()
	}()
	return l.id
}

// SubscribeSerial registers a serial listener on the process-global Bus.
func SubscribeSerial(topics []string, fn interface{}, config SerialConfig) string {
	return sb.SubscribeSerial(topics, fn, config)
}

func Subscribe(topic string, fn interface{}) string {
	l := NewListener(topic, fn, false, false)
	sb.Subscribe(l)
	return l.id
}

func SubscribeOnce(topic string, fn interface{}) string {
	l := NewListener(topic, fn, false, true)

	sb.Subscribe(l)
	return l.id
}

func SubscribeWithBlock(topic string, fn interface{}) string {
	l := NewListener(topic, fn, true, false)

	sb.Subscribe(l)
	return l.id
}

func Unsubscribe(lid string) {
	sb.Unsubscribe(lid)
}

func Publish(topic string, args ...interface{}) {
	sb.Publish(topic, args...)
}

// Close unsubscribes all listeners from the process-global Bus.
func Close() {
	sb.Close()
}

// Wait waits for serial listeners on the process-global Bus to finish.
func Wait() {
	sb.Wait()
}

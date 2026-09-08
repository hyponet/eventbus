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
	b.listeners[l.id] = l
	b.exchange.add(l.topic, l.id)
	if l.serial {
		b.serials[l.id] = l
	}
	b.mux.Unlock()
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

type serialConfig struct {
	buffer int
	block  bool
	onDrop func(topic string, dropped uint64)
}

// SerialOption configures a serial listener created by SubscribeSerial.
type SerialOption func(*serialConfig)

// WithBuffer sets the FIFO queue size of a serial listener.
// The default is 256; n <= 0 keeps the default.
func WithBuffer(n int) SerialOption {
	return func(c *serialConfig) {
		if n > 0 {
			c.buffer = n
		}
	}
}

// WithBlock makes publishers block while the serial listener's queue is
// full, instead of applying the default drop-oldest policy. Use it for
// consumers that must not lose messages (e.g. persistence) and can keep
// up with the publisher. A handler must not synchronously publish to its own
// full queue because it is the only consumer that can free queue capacity.
func WithBlock() SerialOption {
	return func(c *serialConfig) { c.block = true }
}

// WithDropCallback registers a callback invoked every time the
// drop-oldest policy discards a message, with the topic and the
// cumulative drop count. Concurrent publishers may invoke the callback
// concurrently, so the callback must synchronize its own mutable state.
func WithDropCallback(fn func(topic string, dropped uint64)) SerialOption {
	return func(c *serialConfig) { c.onDrop = fn }
}

// SubscribeSerial registers fn on topic with guaranteed in-order
// delivery: each publish is enqueued on a per-listener bounded FIFO
// queue drained by a dedicated goroutine, so handler calls happen in
// publish order. When the queue is full the oldest queued message is
// dropped (unless WithBlock is given).
//
// SubscribeSerial does not support one-shot semantics.
func (b *Bus) SubscribeSerial(topic string, fn interface{}, opts ...SerialOption) string {
	c := serialConfig{buffer: defaultSerialBuffer}
	for _, opt := range opts {
		opt(&c)
	}
	l := NewListener(topic, fn, false, false)
	l.serial = true
	l.capacity = c.buffer
	l.queue = make([][]interface{}, 0, c.buffer)
	l.blockOnFull = c.block
	l.onDrop = c.onDrop
	l.done = make(chan struct{})
	l.cond = sync.NewCond(&l.mux)
	b.Subscribe(l)
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
func SubscribeSerial(topic string, fn interface{}, opts ...SerialOption) string {
	return sb.SubscribeSerial(topic, fn, opts...)
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

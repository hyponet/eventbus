package bus

import "sync"

var (
	evb *eventbus
)

func init() {
	evb = &eventbus{
		listeners: map[string]*listener{},
		exchange:  newExchange(),
	}
}

type eventbus struct {
	listeners map[string]*listener
	exchange  *exchange
	mux       sync.RWMutex
}

func (b *eventbus) register(l *listener) {
	b.mux.Lock()
	b.listeners[l.id] = l
	b.exchange.add(l.topic, l.id)
	b.mux.Unlock()
}

func (b *eventbus) unregister(lID string) {
	b.mux.Lock()
	b.unregisterWithLock(lID)
	b.mux.Unlock()
}

func (b *eventbus) unregisterWithLock(lID string) {
	delete(b.listeners, lID)
	b.exchange.remove(lID)
}

func (b *eventbus) publish(topic string, args ...interface{}) {
	var needDo []*listener
	b.mux.Lock()
	lIDs := b.exchange.route(topic)
	for i, lID := range lIDs {
		needDo = append(needDo, b.listeners[lID])
		if needDo[i].once {
			b.unregisterWithLock(lID)
		}
	}
	b.mux.Unlock()

	for i := range needDo {
		l := needDo[i]
		go func() {
			l.call(args...)
		}()
	}
}

func Register(topic string, fn interface{}) (string, error) {
	l, err := buildNewListener(topic, fn, false, false)
	if err != nil {
		return "", err
	}

	evb.register(l)
	return l.id, nil
}

func RegisterOnce(topic string, fn interface{}) (string, error) {
	l, err := buildNewListener(topic, fn, false, true)
	if err != nil {
		return "", err
	}

	evb.register(l)
	return l.id, nil
}

func RegisterWithBlock(topic string, fn interface{}) (string, error) {
	l, err := buildNewListener(topic, fn, true, false)
	if err != nil {
		return "", err
	}

	evb.register(l)
	return l.id, nil
}

func Unregister(lID string) {
	evb.unregister(lID)
}

func Publish(topic string, args ...interface{}) {
	evb.publish(topic, args...)
}

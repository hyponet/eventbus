package eventbus

type hook func(args ...interface{}) error
type errHandle func(ID string, topic Topic, err error)

type listener struct {
	ID        string
	Topic     Topic
	Fn        hook
	ErrHandle errHandle
	Block     bool
}

func (l *listener) do(args ...interface{}) {
	f := func() {
		if err := l.Fn(args...); err != nil {
			if l.ErrHandle != nil {
				l.ErrHandle(l.ID, l.Topic, err)
				return
			}
		}
	}

	if l.Block {
		f()
		return
	}
	go func() {
		f()
	}()
}

func NewSimpleListener(ID string, hookFn hook) *listener {
	return &listener{
		ID:    ID,
		Fn:    hookFn,
		Block: true,
	}
}

func NewBlockListener(ID string, hookFn hook) *listener {
	return &listener{
		ID:    ID,
		Fn:    hookFn,
		Block: true,
	}
}

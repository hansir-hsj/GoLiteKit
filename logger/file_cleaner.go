package logger

// fileCleaner runs old-log cleanup on a single background goroutine. Kicks
// that arrive while a cleanup is pending are merged, and close waits for the
// goroutine to finish, running a pending cleanup first.
type fileCleaner struct {
	clean   func()
	trigger chan struct{}
	stop    chan struct{}
	done    chan struct{}
}

// newFileCleaner returns nil (a no-op cleaner) when clean is nil.
func newFileCleaner(clean func()) *fileCleaner {
	if clean == nil {
		return nil
	}
	c := &fileCleaner{
		clean:   clean,
		trigger: make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go c.loop()
	return c
}

func (c *fileCleaner) loop() {
	defer close(c.done)
	for {
		select {
		case <-c.trigger:
			c.clean()
		case <-c.stop:
			select {
			case <-c.trigger:
				c.clean()
			default:
			}
			return
		}
	}
}

// kick schedules a cleanup without blocking.
func (c *fileCleaner) kick() {
	if c == nil {
		return
	}
	select {
	case c.trigger <- struct{}{}:
	default: // one is already pending
	}
}

// close stops the goroutine and waits for it. It must be called once.
func (c *fileCleaner) close() {
	if c == nil {
		return
	}
	close(c.stop)
	<-c.done
}

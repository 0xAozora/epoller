//go:build darwin || netbsd || freebsd || openbsd || dragonfly
// +build darwin netbsd freebsd openbsd dragonfly

package epoller

import (
	"net"
	"sync"
	"syscall"
)

type Event = int16

const (
	EPOLLIN      Event = syscall.EVFILT_READ
	EPOLLOUT           = syscall.EVFILT_WRITE
	EPOLLONESHOT       = syscall.EV_ONESHOT // For compatibility
)

type epoll struct {
	fd    int
	event Event
	flags uint16

	connBufferSize int
	changes        []syscall.Kevent_t
	conns          map[uint64]net.Conn
	mu             *sync.RWMutex
	connbuf        []net.Conn
	events         []syscall.Kevent_t

	polling bool
}

func NewPoller(connBufferSize int, event Event) (Poller, error) {

	p, err := syscall.Kqueue()
	if err != nil {
		panic(err)
	}
	_, err = syscall.Kevent(p, []syscall.Kevent_t{{
		Ident:  0,
		Filter: syscall.EVFILT_USER,
		Flags:  syscall.EV_ADD | syscall.EV_CLEAR,
	}}, nil, nil)
	if err != nil {
		panic(err)
	}

	var flags uint16

	if event == 0 {
		event = EPOLLIN
		flags = syscall.EV_EOF
	}
	if event&syscall.EV_ONESHOT != 0 {
		flags = syscall.EV_ONESHOT
		event = event &^ syscall.EV_ONESHOT
	}

	return &epoll{
		fd:             p,
		event:          event,
		flags:          flags,
		mu:             &sync.RWMutex{},
		conns:          make(map[uint64]net.Conn),
		connbuf:        make([]net.Conn, connBufferSize),
		events:         make([]syscall.Kevent_t, connBufferSize),
		connBufferSize: connBufferSize,
	}, nil
}

func (e *epoll) Close(closeConns bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if closeConns {
		for _, conn := range e.conns {
			conn.Close()
		}
	}

	e.conns = nil
	e.changes = nil
	e.events = e.events[:0]
	e.connbuf = e.connbuf[:0]

	return syscall.Close(e.fd)
}

func (e *epoll) Add(conn net.Conn, fd uint64) error {

	event := syscall.Kevent_t{
		Ident: fd, Flags: syscall.EV_ADD | e.flags, Filter: e.event,
	}

	e.mu.Lock()
	if !e.polling {
		e.changes = append(e.changes, event)
	} else {
		e.mu.Unlock()
		syscall.Kevent(e.fd, []syscall.Kevent_t{{
			Ident:  0,
			Filter: syscall.EVFILT_USER,
			Fflags: syscall.NOTE_TRIGGER,
		}, event}, nil, nil)
		e.mu.Lock()
	}

	e.conns[fd] = conn
	e.mu.Unlock()
	return nil
}

func (e *epoll) Remove(fd uint64) error {

	event := syscall.Kevent_t{
		Ident: fd, Flags: syscall.EV_DELETE, Filter: e.event,
	}

	e.mu.Lock()
	if !e.polling {
		e.changes = append(e.changes, event)
	} else {
		e.mu.Unlock()
		syscall.Kevent(e.fd, []syscall.Kevent_t{{
			Ident:  0,
			Filter: syscall.EVFILT_USER,
			Fflags: syscall.NOTE_TRIGGER,
		}, event}, nil, nil)
		e.mu.Lock()
	}

	delete(e.conns, fd)
	e.mu.Unlock()
	return nil
}

func (e *epoll) Wait(count int) ([]net.Conn, error) {

	if count > cap(e.events) {
		e.events = make([]syscall.Kevent_t, count)
	}

	e.mu.RLock()
	changes := e.changes
	e.changes = e.changes[:0] // Allows to reuse the space
	e.polling = true
	e.mu.RUnlock()

retry:
	n, err := syscall.Kevent(e.fd, changes, e.events, nil)
	if err != nil {
		if err == syscall.EINTR {
			goto retry
		}
		return nil, err
	}
	e.polling = false

	var conns []net.Conn
	if e.connBufferSize == 0 {
		conns = make([]net.Conn, 0, n)
	} else {
		conns = e.connbuf[:0]
	}

	e.mu.RLock()
	for i := 0; i < n; i++ {
		conn := e.conns[e.events[i].Ident]
		conns = append(conns, conn)
	}
	e.mu.RUnlock()

	return conns, nil
}

func (e *epoll) Size() int {
	return len(e.conns)
}

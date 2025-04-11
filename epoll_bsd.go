//go:build darwin || netbsd || freebsd || openbsd || dragonfly
// +build darwin netbsd freebsd openbsd dragonfly

package epoller

import (
	"net"
	"sync"
	"syscall"
)

type epoll struct {
	fd          int
	changes     []syscall.Kevent_t
	connections map[uint64]net.Conn
	mu          *sync.RWMutex
	connbuf     []net.Conn
	events      []syscall.Kevent_t

	polling bool
}

func NewPoller() (Poller, error) {
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

	return &epoll{
		fd:          p,
		mu:          &sync.RWMutex{},
		connbuf:     make([]net.Conn, 128, 128),
		events:      make([]syscall.Kevent_t, 128, 128),
		connections: make(map[uint64]net.Conn),
	}, nil
}

func NewPollerWithBuffer(count int) (Poller, error) {
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

	return &epoll{
		fd:          p,
		mu:          &sync.RWMutex{},
		connections: make(map[uint64]net.Conn),
		connbuf:     make([]net.Conn, count, count),
		events:      make([]syscall.Kevent_t, count, count),
	}, nil
}

func (e *epoll) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.connections = nil
	e.changes = nil
	return syscall.Close(e.fd)
}

func (e *epoll) Add(conn net.Conn, fd uint64) error {

	event := syscall.Kevent_t{
		Ident: fd, Flags: syscall.EV_ADD | syscall.EV_EOF, Filter: syscall.EVFILT_READ,
	}

	e.mu.Lock()

	if !e.polling {
		e.changes = append(e.changes, event)
	} else {
		syscall.Kevent(e.fd, []syscall.Kevent_t{{
			Ident:  0,
			Filter: syscall.EVFILT_USER,
			Fflags: syscall.NOTE_TRIGGER,
		}, event}, nil, nil)
	}

	e.connections[fd] = conn
	e.mu.Unlock()
	return nil
}

func (e *epoll) Remove(fd uint64) error {

	event := syscall.Kevent_t{
		Ident: fd, Flags: syscall.EV_DELETE, Filter: syscall.EVFILT_READ,
	}

	e.mu.Lock()

	if !e.polling {
		e.changes = append(e.changes, event)
	} else {
		syscall.Kevent(e.fd, []syscall.Kevent_t{{
			Ident:  0,
			Filter: syscall.EVFILT_USER,
			Fflags: syscall.NOTE_TRIGGER,
		}, event}, nil, nil)
	}

	delete(e.connections, fd)
	e.mu.Unlock()
	return nil
}

func (e *epoll) Wait(count int) ([]net.Conn, error) {
	events := make([]syscall.Kevent_t, count, count)

	e.mu.RLock()
	changes := e.changes
	e.changes = e.changes[:0] // Allows to reuse the space
	e.polling = true
	e.mu.RUnlock()

retry:
	n, err := syscall.Kevent(e.fd, changes, events, nil)
	if err != nil {
		if err == syscall.EINTR {
			goto retry
		}
		return nil, err
	}
	e.polling = false

	var connections = make([]net.Conn, 0, n)
	e.mu.RLock()
	for i := 0; i < n; i++ {
		conn := e.connections[events[i].Ident]
		if (events[i].Flags & syscall.EV_EOF) == syscall.EV_EOF {
			conn.Close()
		}
		connections = append(connections, conn)
	}
	e.mu.RUnlock()

	return connections, nil
}

func (e *epoll) WaitWithBuffer() ([]net.Conn, error) {
	e.mu.RLock()
	changes := e.changes
	e.changes = e.changes[:0]
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

	var connections = e.connbuf[:0]
	e.mu.RLock()
	for i := 0; i < n; i++ {
		conn := e.connections[e.events[i].Ident]
		if (e.events[i].Flags & syscall.EV_EOF) == syscall.EV_EOF {
			conn.Close()
		}
		connections = append(connections, conn)
	}
	e.mu.RUnlock()
	return connections, nil
}

func (e *epoll) WaitChan(count int) <-chan []net.Conn {
	ch := make(chan []net.Conn)
	go func() {
		for {
			conns, err := e.Wait(count)
			if err != nil {
				close(ch)
				return
			}

			if len(conns) == 0 {
				continue
			}

			ch <- conns
		}
	}()
	return ch
}

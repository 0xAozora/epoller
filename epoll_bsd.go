//go:build darwin || netbsd || freebsd || openbsd || dragonfly
// +build darwin netbsd freebsd openbsd dragonfly

package epoller

import (
	"errors"
	"net"
	"sync"
	"syscall"
)

var _ Poller = (*Epoll)(nil)

// Epoll is a epoll based poller.
type Epoll struct {
	fd int

	connBufferSize int
	mu             *sync.RWMutex
	changes        []syscall.Kevent_t
	conns          map[int]net.Conn
	connbuf        []net.Conn

	polling bool
}

// NewPoller creates a new poller instance.
func NewPoller(connBufferSize int) (*Epoll, error) {
	return newPollerWithBuffer(connBufferSize)
}

// newPollerWithBuffer creates a new poller instance with buffer size.
func newPollerWithBuffer(count int) (*Epoll, error) {
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

	return &Epoll{
		fd:             p,
		connBufferSize: count,
		mu:             &sync.RWMutex{},
		conns:          make(map[int]net.Conn),
		connbuf:        make([]net.Conn, count),
	}, nil
}

// Close closes the poller.
func (e *Epoll) Close(closeConns bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if closeConns {
		for _, conn := range e.conns {
			conn.Close()
		}
	}

	e.conns = nil
	e.changes = nil
	e.connbuf = e.connbuf[:0]

	return syscall.Close(e.fd)
}

// Add adds a network connection to the poller.
func (e *Epoll) Add(conn net.Conn) error {
	conn = newConnImpl(conn)
	fd := socketFD(conn)
	if e := syscall.SetNonblock(int(fd), true); e != nil {
		return errors.New("udev: unix.SetNonblock failed")
	}

	event := syscall.Kevent_t{
		Ident: uint64(fd), Flags: syscall.EV_ADD | syscall.EV_EOF, Filter: syscall.EVFILT_READ,
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

	e.conns[fd] = conn
	e.mu.Unlock()

	return nil
}

// Remove removes a connection from the poller.
// If close is true, the connection will be closed.
func (e *Epoll) Remove(conn net.Conn) error {
	fd := socketFD(conn)

	event := syscall.Kevent_t{
		Ident: uint64(fd), Flags: syscall.EV_DELETE, Filter: syscall.EVFILT_READ,
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

	delete(e.conns, fd)
	e.mu.Unlock()

	return nil
}

// Wait waits for events and returns the connections.
func (e *Epoll) Wait(count int) ([]net.Conn, error) {
	events := make([]syscall.Kevent_t, count)

	e.mu.RLock()
	changes := e.changes
	// Its "ok" to write in an RLock here, as we are the only ones reading those
	// And we prevent Add and Remove to access them
	e.changes = e.changes[:0]
	e.polling = true
	e.mu.RUnlock()

retry:
	n, err := syscall.Kevent(e.fd, changes, events, nil)
	if err != nil {
		if err == syscall.EINTR {
			goto retry
		}
		e.polling = false
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
		conn := e.conns[int(events[i].Ident)]
		if conn != nil {
			// issue #11: don't close the connection here because maybe data needs to drain
			//
			// if (events[i].Flags & syscall.EV_EOF) == syscall.EV_EOF {
			// 	conn.Close()
			// }
			conns = append(conns, conn)
		}
	}
	e.mu.RUnlock()

	return conns, nil
}

func (e *Epoll) Size() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.conns)
}

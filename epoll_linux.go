//go:build linux
// +build linux

package epoller

import (
	"net"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

type Event = uint32

const (
	EPOLLIN      Event = unix.EPOLLIN
	EPOLLPRI           = unix.EPOLLPRI
	EPOLLOUT           = unix.EPOLLOUT
	EPOLLERR           = unix.EPOLLERR
	EPOLLHUP           = unix.EPOLLHUP
	EPOLLRDNORM        = unix.EPOLLRDNORM
	EPOLLRDBAND        = unix.EPOLLRDBAND
	EPOLLWRNORM        = unix.EPOLLWRNORM
	EPOLLWRBAND        = unix.EPOLLWRBAND
	EPOLLMSG           = unix.EPOLLMSG
	EPOLLRDHUP         = unix.EPOLLRDHUP
	EPOLLONESHOT       = unix.EPOLLONESHOT
)

type epoll struct {
	fd    int
	event Event

	conns          map[uint64]net.Conn
	lock           *sync.RWMutex
	connbuf        []net.Conn
	connBufferSize int
	events         []unix.EpollEvent
}

func NewPoller(connBufferSize int, event Event) (Poller, error) {
	fd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}

	if event == 0 {
		event = EPOLLIN | EPOLLHUP
	}

	return &epoll{
		fd:             fd,
		event:          event,
		lock:           &sync.RWMutex{},
		conns:          make(map[uint64]net.Conn),
		events:         make([]unix.EpollEvent, connBufferSize),
		connbuf:        make([]net.Conn, connBufferSize),
		connBufferSize: connBufferSize,
	}, nil
}

func (e *epoll) Close(closeConns bool) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	if closeConns {
		for _, conn := range e.conns {
			conn.Close()
		}
	}

	e.conns = nil
	e.connbuf = e.connbuf[:0]
	e.events = e.events[:0]
	return unix.Close(e.fd)
}

func (e *epoll) Add(conn net.Conn, fd uint64) error {
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_ADD, int(fd), &unix.EpollEvent{Events: e.event, Fd: int32(fd)})
	if err != nil {
		return err
	}
	e.lock.Lock()
	e.conns[fd] = conn
	e.lock.Unlock()
	return nil
}

func (e *epoll) Remove(fd uint64) error {
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_DEL, int(fd), nil)
	if err != nil {
		return err
	}
	e.lock.Lock()
	delete(e.conns, fd)
	e.lock.Unlock()
	return nil
}

func (e *epoll) Wait(count int) ([]net.Conn, error) {

	if count > cap(e.events) {
		e.events = make([]unix.EpollEvent, count)
	}

retry:
	n, err := unix.EpollWait(e.fd, e.events, -1)
	if err != nil {
		if err == unix.EINTR {
			goto retry
		}
		return nil, err
	}

	var conns []net.Conn
	if e.connBufferSize == 0 {
		conns = make([]net.Conn, 0, n)
	} else {
		conns = e.connbuf[:0]
	}

	e.lock.RLock()
	for i := 0; i < n; i++ {
		conn := e.conns[uint64(e.events[i].Fd)]
		if conn != nil {
			conns = append(conns, conn)
		}
	}
	e.lock.RUnlock()

	return conns, nil
}

func (e *epoll) Size() int {
	e.lock.RLock()
	defer e.lock.RUnlock()

	return len(e.conns)
}

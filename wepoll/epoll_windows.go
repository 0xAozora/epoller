//go:build windows && cgo
// +build windows,cgo

package wepoll

//#cgo windows LDFLAGS: -lws2_32 -lwsock32
//#include"wepoll.h"
import "C"
import (
	"errors"

	"net"
	"sync"
)

type Event C.uint

const (
	EPOLLIN      Event = C.EPOLLIN
	EPOLLPRI           = C.EPOLLPRI
	EPOLLOUT           = C.EPOLLOUT
	EPOLLERR           = C.EPOLLERR
	EPOLLHUP           = C.EPOLLHUP
	EPOLLRDNORM        = C.EPOLLRDNORM
	EPOLLRDBAND        = C.EPOLLRDBAND
	EPOLLWRNORM        = C.EPOLLWRNORM
	EPOLLWRBAND        = C.EPOLLWRBAND
	EPOLLMSG           = C.EPOLLMSG /* Never reported. */
	EPOLLRDHUP         = C.EPOLLRDHUP
	EPOLLONESHOT       = C.EPOLLONESHOT
)

type Epoll struct {
	fd    C.uintptr_t
	event Event

	conns          map[int]net.Conn
	lock           *sync.RWMutex
	connbuf        []net.Conn
	events         []C.epoll_event
	connBufferSize int
}

func NewPoller(connBufferSize int, event Event) (*Epoll, error) {
	fd := C.epoll_create1(0)

	if fd == 0 {
		return nil, errors.New("epoll_create1 error")
	}

	if event == 0 {
		event = C.EPOLLIN | C.EPOLLHUP
	}

	return &Epoll{
		fd:             fd,
		event:          event,
		lock:           &sync.RWMutex{},
		conns:          make(map[int]net.Conn),
		connbuf:        make([]net.Conn, connBufferSize),
		events:         make([]C.epoll_event, connBufferSize),
		connBufferSize: connBufferSize,
	}, nil
}

func (e *Epoll) Close(closeConns bool) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	if closeConns {
		for _, conn := range e.conns {
			conn.Close()
		}
	}

	e.events = e.events[:0]
	e.connbuf = e.connbuf[:0]

	e.conns = nil
	i := C.epoll_close(e.fd)
	if i == 0 {
		return nil
	} else {
		return errors.New(" an error occurred on epoll.close ")
	}
}

func (e *Epoll) Add(conn net.Conn, f uint64) error {
	// Extract file descriptor associated with the connection
	fd := C.SOCKET(f)
	var ev C.epoll_event
	ev = C.set_epoll_event(C.uint32_t(e.event), C.SOCKET(fd))
	err := C.epoll_ctl(e.fd, C.EPOLL_CTL_ADD, C.SOCKET(fd), &ev)
	if err == -1 {
		return errors.New("C.EPOLL_CTL_ADD error ")
	}
	e.lock.Lock()
	e.conns[int(fd)] = conn
	e.lock.Unlock()
	return nil
}

func (e *Epoll) Remove(f uint64) error {

	fd := C.SOCKET(f)
	var ev C.epoll_event
	err := C.epoll_ctl(e.fd, C.EPOLL_CTL_DEL, C.SOCKET(fd), &ev)
	if err == -1 {
		return errors.New("C.EPOLL_CTL_DEL error ")
	}
	e.lock.Lock()
	delete(e.conns, int(fd))
	e.lock.Unlock()
	return nil
}

func (e *Epoll) Wait(count int) ([]net.Conn, error) {

	if count > cap(e.events) {
		e.events = make([]C.epoll_event, count)
	}

	n := C.epoll_wait(e.fd, &e.events[0], C.int(count), -1)
	if n == -1 {
		return nil, errors.New("C.epoll_wait error")
	}

	var conns []net.Conn
	if e.connBufferSize == 0 {
		conns = make([]net.Conn, 0, n)
	} else {
		conns = e.connbuf[:0]
	}

	e.lock.RLock()
	for i := 0; i < int(n); i++ {
		fd := C.get_epoll_event(e.events[i])
		conn := e.conns[int(fd)]
		conns = append(conns, conn)
	}
	e.lock.RUnlock()

	return conns, nil
}

func (e *Epoll) Size() int {
	return len(e.conns)
}

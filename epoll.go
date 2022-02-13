package epoller

import (
	"net"
	"unsafe"
)

type Poller interface {
	Add(conn net.Conn) error
	Remove(conn net.Conn) error
	Wait(count int) ([]net.Conn, error)
	WaitWithBuffer() ([]net.Conn, error)
	WaitChan(count int) <-chan []net.Conn
	Close() error
}

// *net.TCPListener | *net.TCPConn
func getFD(p unsafe.Pointer) int64 {
	pfd := *(*unsafe.Pointer)(p)
	return *(*int64)(unsafe.Pointer(uintptr(pfd) + uintptr(16)))
}
package epoller

import (
	"net"
	"unsafe"
)

type Poller interface {
	Add(conn net.Conn, fd uint64) error
	Remove(fd uint64) error
	Wait(count int) ([]net.Conn, error)
	Close(closeConns bool) error
}

// *net.TCPListener | *net.TCPConn
func GetFD(p unsafe.Pointer) uint64 {
	pfd := *(*unsafe.Pointer)(p)
	return *(*uint64)(unsafe.Pointer(uintptr(pfd) + uintptr(16)))
}

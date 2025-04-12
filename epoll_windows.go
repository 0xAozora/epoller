//go:build windows && cgo
// +build windows,cgo

package epoller

import (
	"github.com/0xAozora/epoller/wepoll"
)

// TODO: Think of a better way to expose the events
type Event = wepoll.Event

const (
	EPOLLIN      wepoll.Event = wepoll.EPOLLIN
	EPOLLPRI                  = wepoll.EPOLLPRI
	EPOLLOUT                  = wepoll.EPOLLOUT
	EPOLLERR                  = wepoll.EPOLLERR
	EPOLLHUP                  = wepoll.EPOLLHUP
	EPOLLRDNORM               = wepoll.EPOLLRDNORM
	EPOLLRDBAND               = wepoll.EPOLLRDBAND
	EPOLLWRNORM               = wepoll.EPOLLWRNORM
	EPOLLWRBAND               = wepoll.EPOLLWRBAND
	EPOLLMSG                  = wepoll.EPOLLMSG /* Never reported. */
	EPOLLRDHUP                = wepoll.EPOLLRDHUP
	EPOLLONESHOT              = wepoll.EPOLLONESHOT
)

type epoll = wepoll.Epoll

func NewPoller(connBufferSize int, event Event) (Poller, error) {
	return wepoll.NewPoller(connBufferSize, event)
}

//go:build windows && cgo
// +build windows,cgo

package epoller

import (
	"github.com/0xAozora/epoller/wepoll"
)

type epoll = wepoll.Epoll

func NewPoller(connBufferSize int) (Poller, error) {
	return wepoll.NewPoller(connBufferSize)
}

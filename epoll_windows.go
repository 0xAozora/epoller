//go:build windows && cgo
// +build windows,cgo

package epoller

import (
	"github.com/0xAozora/epoller/wepoll"
)

type epoll = wepoll.Epoll

func NewPoller() (Poller, error) {
	return wepoll.NewPoller()
}

func NewPollerWithBuffer(count int) (Poller, error) {
	return wepoll.NewPollerWithBuffer(count)
}

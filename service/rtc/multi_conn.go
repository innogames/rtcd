// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/ipv4"
)

const (
	receiveMTU = 8192
)

type multiConn struct {
	conns        []*ipv4.PacketConn
	dstIPs       map[string]*ipv4.ControlMessage
	dstIPMu      sync.RWMutex
	addr         net.Addr
	readResultCh chan readResult
	closeCh      chan struct{}
	bufPool      *sync.Pool
	counter      uint64
	wg           sync.WaitGroup
}

type readResult struct {
	n    int
	addr net.Addr
	err  error
	buf  []byte
}

func newMultiConn(conns []*ipv4.PacketConn) (*multiConn, error) {
	if len(conns) == 0 {
		return nil, errors.New("conns should not be empty")
	}
	for _, conn := range conns {
		if conn == nil {
			return nil, errors.New("invalid nil conn")
		}
	}
	var mc multiConn
	mc.conns = conns
	mc.dstIPs = make(map[string]*ipv4.ControlMessage, len(conns))
	mc.addr = conns[0].LocalAddr()
	mc.readResultCh = make(chan readResult)
	mc.closeCh = make(chan struct{})
	mc.bufPool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, receiveMTU)
		},
	}
	mc.wg.Add(len(conns))
	for _, conn := range conns {
		go mc.reader(conn)
	}
	return &mc, nil
}

func (mc *multiConn) reader(conn *ipv4.PacketConn) {
	defer mc.wg.Done()
	var res readResult
	var cm *ipv4.ControlMessage

	for {
		res.buf = mc.bufPool.Get().([]byte)
		res.n, cm, res.addr, res.err = conn.ReadFrom(res.buf)

		// Track the destination IP (ourselves) of the incoming packet,
		// so we can later send a packet originating from that same IP
		if res.addr != nil && cm != nil {
			mc.dstIPMu.Lock()
			mc.dstIPs[res.addr.String()] = &ipv4.ControlMessage{Src: cm.Dst}
			mc.dstIPMu.Unlock()
		}

		select {
		case mc.readResultCh <- res:
		case <-mc.closeCh:
			return
		}
		if os.IsTimeout(res.err) {
			continue
		} else if res.err != nil {
			break
		}
	}
}

func (mc *multiConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	res := <-mc.readResultCh
	copy(p, res.buf[:res.n])
	mc.bufPool.Put(res.buf)
	return res.n, res.addr, res.err
}

func (mc *multiConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	// Simple round-robin to equally distribute the writes among the connections.
	idx := (atomic.AddUint64(&mc.counter, 1) - 1) % uint64(len(mc.conns))

	mc.dstIPMu.RLock()
	cm, ok := mc.dstIPs[addr.String()]
	mc.dstIPMu.RUnlock()
	if !ok {
		cm = &ipv4.ControlMessage{}
	}

	return mc.conns[idx].WriteTo(p, cm, addr)
}

func (mc *multiConn) Close() error {
	var err error
	close(mc.closeCh)
	for _, conn := range mc.conns {
		err = conn.Close()
	}
	mc.wg.Wait()
	close(mc.readResultCh)
	return err
}

func (mc *multiConn) LocalAddr() net.Addr {
	return mc.addr
}

func (mc *multiConn) SetDeadline(t time.Time) error {
	var err error
	for _, conn := range mc.conns {
		err = conn.SetDeadline(t)
	}
	return err
}

func (mc *multiConn) SetReadDeadline(t time.Time) error {
	var err error
	for _, conn := range mc.conns {
		err = conn.SetReadDeadline(t)
	}
	return err
}

func (mc *multiConn) SetWriteDeadline(t time.Time) error {
	var err error
	for _, conn := range mc.conns {
		err = conn.SetWriteDeadline(t)
	}
	return err
}

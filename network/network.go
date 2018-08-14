/*
PASL - Personalized Accounts & Secure Ledger

Copyright (C) 2018 PASL Project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package network

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pasl-project/pasl/utils"

	"github.com/cevaris/ordered_map"
	"github.com/tidwall/evio"
)

type Server struct {
	Listen   string
	MaxPeers int
}

type Config struct {
	ListenAddrs    []string
	MaxIncoming    uint32
	MaxOutgoing    uint32
	TimeoutConnect time.Duration
}

type Node interface {
	AddPeer(address Address) bool
	GetPeersByType(addressType AddressType) map[Address]*Peer
}

type Peer struct {
	Address              Address
	LastConnectTimestamp uint32
	Attempts             int
	Errors               int
}

type Manager interface {
	OnOpen(address string, transport io.WriteCloser, isOutgoing bool) (interface{}, error)
	OnData(interface{}, []byte) error
	OnClose(interface{})
}

type connection struct {
	destination    *Peer
	context        interface{}
	sendBuffer     []byte
	sendBufferLen  int
	sendBufferLock sync.Mutex
	closing        bool
	onWrite        func([]byte) (int, error)
	onClose        func() error
}

type nodeInternal struct {
	Config          Config
	Server          *evio.Server
	ReadyEvent      chan bool
	StopEvent       chan<- bool
	PeersQueue      *ordered_map.OrderedMap
	PeersInProgress map[int]*Peer
	PeersLock       sync.RWMutex
	Incoming        int32
	Outgoing        int32
	Connected       map[int]*connection
}

func WithNode(config Config, manager Manager, fn func(node Node) error) error {
	tickDelay := time.Duration(1) * time.Second

	ready := make(chan bool, 1)
	finish := make(chan bool, 1)

	var node *nodeInternal = nil

	var events evio.Events
	events.Serving = func(srv evio.Server) (action evio.Action) {
		node = &nodeInternal{
			Config:          config,
			Server:          &srv,
			ReadyEvent:      ready,
			StopEvent:       finish,
			PeersQueue:      ordered_map.NewOrderedMap(),
			PeersInProgress: make(map[int]*Peer),
			Connected:       make(map[int]*connection),
		}
		ready <- true
		utils.Tracef("Node listening %v", config.ListenAddrs)
		return
	}
	events.Opened = func(id int, info evio.Info) (out []byte, opts evio.Options, action evio.Action) {
		if info.Closing {
			return
		}

		var isOutgoing bool
		func() {
			node.PeersLock.Lock()
			defer node.PeersLock.Unlock()

			peer := node.PeersInProgress[id]
			if peer != nil {
				delete(node.PeersInProgress, id)
			}
			isOutgoing = peer != nil
			conn := &connection{
				destination:   peer,
				sendBuffer:    nil,
				sendBufferLen: 0,
			}
			conn.onWrite = func(data []byte) (int, error) {
				conn.sendBufferLock.Lock()

				prevLength := conn.sendBufferLen
				dataLength := len(data)
				conn.sendBufferLen = prevLength + dataLength
				newBuffer := make([]byte, conn.sendBufferLen)
				if prevLength > 0 {
					copy(newBuffer, conn.sendBuffer)
				}
				copy(newBuffer[prevLength:], data)
				conn.sendBuffer = newBuffer

				conn.sendBufferLock.Unlock()

				if !node.Server.Wake(id) {
					return 0, errors.New("Failed to wake connection")
				}

				return dataLength, nil
			}
			conn.onClose = func() error {
				if conn.closing != true {
					conn.closing = true
					return nil
				}
				return errors.New("Attempt to Close already closed connection")
			}
			context, err := manager.OnOpen("tcp://"+info.RemoteAddr.String(), conn, isOutgoing)
			if err != nil {
				utils.Tracef("%v", err)
				action = evio.Close
			}
			conn.context = context

			node.Connected[id] = conn
		}()

		if isOutgoing {
			atomic.AddInt32(&node.Outgoing, 1)
		} else if uint32(atomic.AddInt32(&node.Incoming, 1)) > node.Config.MaxIncoming {
			action = evio.Close
		}

		node.Updated()

		return
	}
	events.Closed = func(id int, err error) (action evio.Action) {
		var ok bool
		var isOutgoing bool
		if _, ok = node.Connected[id]; ok {
			manager.OnClose(node.Connected[id].context)

			isOutgoing = node.Connected[id].destination != nil
			if isOutgoing {
				atomic.AddInt32(&node.Outgoing, -1)
			} else {
				atomic.AddInt32(&node.Incoming, -1)
			}
		}

		func() {
			node.PeersLock.Lock()
			defer node.PeersLock.Unlock()

			if ok {
				if isOutgoing {
					node.PeersQueue.Set(node.Connected[id].destination.Address, node.Connected[id].destination)
				}
				delete(node.Connected, id)
			} else {
				node.PeersQueue.Set(node.PeersInProgress[id].Address, node.PeersInProgress[id])
				delete(node.PeersInProgress, id)
			}
		}()

		node.Updated()
		return
	}
	events.Data = func(id int, in []byte) (out []byte, action evio.Action) {
		conn := node.Connected[id]
		if in == nil {
			conn.sendBufferLock.Lock()
			defer conn.sendBufferLock.Unlock()
			out = conn.sendBuffer
			conn.sendBufferLen = 0
		} else {
			err := manager.OnData(conn.context, in)
			if err != nil {
				utils.Tracef("Disconnecting peer (%v)", err)
				action = evio.Close
			}
		}

		if conn.closing == true {
			action = evio.Close
		}

		return
	}
	events.Tick = func() (delay time.Duration, action evio.Action) {
		select {
		case <-finish:
			return tickDelay, evio.Shutdown
		default:
			return tickDelay, evio.None
		}
	}

	var err error = nil
	go func() {
		err = evio.Serve(events, config.ListenAddrs...)
		ready <- true
	}()
	<-ready

	if err != nil {
		return err
	}

	defer node.stopAndWait()
	return fn(node)
}

func (node *nodeInternal) GetPeersByType(addressType AddressType) (result map[Address]*Peer) {
	result = make(map[Address]*Peer)

	node.PeersLock.RLock()
	defer node.PeersLock.RUnlock()

	for _, connection := range node.Connected {
		peer := connection.destination
		if peer == nil {
			continue
		}
		address := peer.Address
		if address.GetType() == addressType {
			result[address] = peer
		}
	}

	return
}

func (node *nodeInternal) AddPeer(address Address) bool {
	node.PeersLock.Lock()
	defer node.Updated()
	defer node.PeersLock.Unlock()

	if _, exists := node.PeersQueue.Get(address); exists {
		return false
	}

	node.PeersQueue.Set(address, &Peer{
		Address:              address,
		LastConnectTimestamp: 0,
		Attempts:             0,
		Errors:               0,
	})

	return true
}

func (node *nodeInternal) Updated() {
	node.PeersLock.Lock()
	defer node.PeersLock.Unlock()

	count := (int)(atomic.LoadInt32(&node.Outgoing)) + len(node.PeersInProgress)
	max := (int)(node.Config.MaxOutgoing)
	if count < max {

		toAdd := max - count

		iter := node.PeersQueue.IterFunc()
		for kv, ok := iter(); ok && toAdd > 0; kv, ok = iter() {
			var address Address = kv.Key.(Address)
			var peer *Peer = kv.Value.(*Peer)

			id := node.Server.Dial(address.String(), node.Config.TimeoutConnect)
			if id != 0 {
				node.PeersQueue.Delete(address)
				node.PeersInProgress[id] = peer
			}
			toAdd--
		}
	}
}

func (node *nodeInternal) stopAndWait() {
	node.StopEvent <- true
	<-node.ReadyEvent
}

func (this *connection) Write(p []byte) (n int, err error) {
	if this.closing != true {
		return this.onWrite(p)
	}
	err = errors.New("Attempt to Write to closed connection")
	return
}

func (this *connection) Close() error {
	return this.onClose()
}

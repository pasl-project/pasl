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
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/modern-go/concurrent"
	"github.com/pasl-project/pasl/utils"
)

type Config struct {
	ListenAddr     string
	MaxIncoming    uint32
	MaxOutgoing    uint32
	TimeoutConnect time.Duration
}

type Node interface {
	AddPeer(network, address string) bool
	GetPeersByNetwork(network string) map[string]*Peer
}

type Peer struct {
	Address              string
	LastConnectTimestamp uint32
	ReconnectPenalty     uint32
	Attempts             int
	Errors               int
}

type Manager interface {
	OnOpen(address string, transport io.WriteCloser, isOutgoing bool) (interface{}, error)
	OnData(interface{}, []byte) error
	OnClose(interface{})
}

type nodeInternal struct {
	Config  Config
	Peers   *PeersList
	Manager Manager
}

func WithNode(config Config, manager Manager, fn func(node Node) error) error {
	node := nodeInternal{
		Config:  config,
		Peers:   NewPeersList(),
		Manager: manager,
	}

	l, err := net.Listen("tcp", config.ListenAddr)
	if err != nil {
		return err
	}
	defer l.Close()
	utils.Tracef("Node listening %v", config.ListenAddr)

	handler := concurrent.NewUnboundedExecutor()
	handler.Go(func(ctx context.Context) {
		wg := sync.WaitGroup{}
		defer wg.Wait()

		for {
			select {
			case <-ctx.Done():
				break
			}

			conn, err := l.Accept()
			if err != nil {
				return
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				node.HandleConnection(conn, "tcp://"+conn.RemoteAddr().String(), false)
			}()
		}
	})
	defer handler.StopAndWaitForever()

	return fn(&node)
}

func (node *nodeInternal) GetPeersByNetwork(network string) map[string]*Peer {
	return make(map[string]*Peer)
}

func (node *nodeInternal) AddPeer(network, address string) bool {
	added := node.Peers.Add(address)
	if added {
		node.Updated()
	}
	return added
}

func (node *nodeInternal) Updated() {
	for _, peer := range node.Peers.ScheduleReconnect((int)(node.Config.MaxOutgoing)) {
		go func() {
			node.Peers.SetConnected(peer)
			defer node.Updated()
			defer node.Peers.SetDisconnected(peer)

			conn, err := net.DialTimeout("tcp", peer.Address, node.Config.TimeoutConnect)
			if err != nil {
				utils.Tracef("Connection failed: %v", err)
				return
			}
			node.HandleConnection(conn, "tcp://"+conn.RemoteAddr().String(), true)
		}()
	}
}

func (node *nodeInternal) HandleConnection(conn io.ReadWriteCloser, address string, isOutgoing bool) {
	defer conn.Close()

	context, err := node.Manager.OnOpen(address, conn, isOutgoing)
	if err != nil {
		utils.Tracef("OnOpen failed: %v", err)
		return
	}
	defer node.Manager.OnClose(context)

	buf := make([]byte, 10*1024)
	for {
		read, err := conn.Read(buf)
		if err != nil {
			break
		}
		if err = node.Manager.OnData(context, buf[:read]); err != nil {
			utils.Tracef("OnData failed: %v", err)
			break
		}
	}
}

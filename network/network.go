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
	"fmt"
	"io"
	"net"
	"net/url"
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

type Node struct {
	config Config
	peers  *PeersList
}

type Peer struct {
	Address              string
	LastConnectTimestamp uint32
	ReconnectPenalty     uint32
	LastSeen             uint64
}

type connectionCloser struct {
	conn    net.Conn
	onClose func()
}

type Connection struct {
	Address        string
	Outgoing       bool
	Transport      io.ReadWriteCloser
	OnStateUpdated func()
}

func WithNode(config Config, peers *PeersList, onNewConnection chan<- *Connection, fn func(node Node) error) error {
	node := Node{
		config: config,
		peers:  peers,
	}

	l, err := net.Listen("tcp", config.ListenAddr)
	if err != nil {
		return err
	}
	utils.Tracef("Node listening %v", config.ListenAddr)

	handler := concurrent.NewUnboundedExecutor()
	handler.Go(func(ctx context.Context) {
		wg := sync.WaitGroup{}
		defer wg.Wait()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				break
			}

			conn, err := l.Accept()
			if err != nil {
				return
			}

			onNewConnection <- &Connection{
				Address:        "tcp://" + conn.RemoteAddr().String(),
				Outgoing:       false,
				Transport:      conn,
				OnStateUpdated: nil,
			}
		}
	})
	defer handler.StopAndWaitForever()
	defer l.Close()

	scheduler := concurrent.NewUnboundedExecutor()
	scheduler.Go(func(ctx context.Context) {
		wg := sync.WaitGroup{}
		defer wg.Wait()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(1) * time.Second):
				break
			}

			for _, peer := range node.peers.ScheduleReconnect((int)(node.config.MaxOutgoing)) {
				wg.Add(1)
				go func(peer *Peer) {
					defer wg.Done()

					parsed, err := url.Parse(peer.Address)
					if err != nil {
						return
					}

					d := net.Dialer{Timeout: node.config.TimeoutConnect}
					conn, err := d.DialContext(ctx, "tcp", parsed.Host)
					if err != nil {
						// utils.Tracef("Connection failed: %v", err)
						return
					}

					onNewConnection <- &Connection{
						Address:  "tcp://" + conn.RemoteAddr().String(),
						Outgoing: true,
						Transport: &connectionCloser{
							conn: conn,
							onClose: func() {
								node.peers.SetDisconnected(peer)
							},
						},
						OnStateUpdated: func() { peer.LastSeen = uint64(time.Now().Unix()) },
					}
				}(peer)
			}
		}
	})
	defer scheduler.StopAndWaitForever()

	return fn(node)
}

func (node *Node) GetPeersByNetwork(network string) map[string]Peer {
	return node.peers.GetAllSeen()
}

func (node *Node) AddPeer(address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return err
	}

	if parsed.Scheme != "tcp" {
		return fmt.Errorf("Unsupported network '%v'", parsed.Scheme)
	}

	tcp, err := net.ResolveTCPAddr("tcp", parsed.Host)
	if err != nil {
		return fmt.Errorf("Failed to resolve TCP addresss %s %v", address, err)
	}
	if !tcp.IP.IsGlobalUnicast() {
		return fmt.Errorf("IP Address %s didn't pass the validation", address)
	}

	node.peers.Add("tcp://"+tcp.String(), nil)
	return nil
}

func (node *Node) AddPeerSerialized(serialized []byte) error {
	return node.peers.AddSerialized(serialized)
}

func (c *connectionCloser) Read(p []byte) (n int, err error) {
	return c.conn.Read(p)
}
func (c *connectionCloser) Write(p []byte) (n int, err error) {
	return c.conn.Write(p)
}
func (c *connectionCloser) Close() error {
	c.onClose()
	return c.conn.Close()
}

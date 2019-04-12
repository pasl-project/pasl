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

package pasl

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/modern-go/concurrent"
	"github.com/pasl-project/pasl/blockchain"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/network"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/utils"
)

type event struct {
	source *PascalConnection
}

type eventNewBlock struct {
	event
	safebox.SerializedBlock
	shouldBroadcast bool
}

type eventNewOperation struct {
	event
	tx.CommonOperation
}

type eventConnectionState struct {
	event
	topBlockIndex uint32
}

type syncState int

const (
	synced  syncState = iota
	syncing           = iota
)

type Manager struct {
	blockchain             *blockchain.Blockchain
	blocksUpdates          <-chan safebox.SerializedBlock
	closed                 chan *PascalConnection
	doSync                 *sync.Cond
	doSyncValue            bool
	initializedConnections sync.Map
	nonce                  []byte
	onNewBlock             chan *eventNewBlock
	onNewOperation         chan *eventNewOperation
	onStateUpdate          chan eventConnectionState
	onSyncState            chan syncState
	p2pPort                uint16
	peers                  *network.PeersList
	peerUpdates            chan<- PeerInfo
	prevSyncState          syncState
	timeoutRequest         time.Duration
	txPoolUpdates          <-chan tx.CommonOperation
	waitGroup              sync.WaitGroup
}

func WithManager(
	nonce []byte,
	blockchain *blockchain.Blockchain,
	p2pPort uint16,
	peers *network.PeersList,
	peerUpdates chan<- PeerInfo,
	blocksUpdates <-chan safebox.SerializedBlock,
	txPoolUpdates <-chan tx.CommonOperation,
	timeoutRequest time.Duration,
	callback func(m *Manager) error,
) error {
	manager := &Manager{
		blockchain:     blockchain,
		blocksUpdates:  blocksUpdates,
		closed:         make(chan *PascalConnection),
		doSync:         sync.NewCond(&sync.Mutex{}),
		nonce:          nonce,
		onNewBlock:     make(chan *eventNewBlock),
		onNewOperation: make(chan *eventNewOperation),
		onStateUpdate:  make(chan eventConnectionState),
		onSyncState:    make(chan syncState),
		p2pPort:        p2pPort,
		peers:          peers,
		peerUpdates:    peerUpdates,
		prevSyncState:  syncing,
		timeoutRequest: timeoutRequest,
		txPoolUpdates:  txPoolUpdates,
	}
	defer manager.waitGroup.Wait()

	signalSync := func() {
		manager.doSync.L.Lock()
		manager.doSyncValue = true
		manager.doSync.Broadcast()
		manager.doSync.L.Unlock()
	}
	defer signalSync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.waitGroup.Add(1)
	go func() {
		defer manager.waitGroup.Done()
		for {
			select {
			case block := <-manager.blocksUpdates:
				utils.Tracef("Broadcasting block")
				manager.broadcastBlock(&block, nil)
			case transaction := <-manager.txPoolUpdates:
				utils.Tracef("Broadcasting tx")
				manager.broadcastTx(transaction, nil)
			case <-ctx.Done():
				return
			}
		}
	}()

	manager.waitGroup.Add(1)
	go func() {
		defer manager.waitGroup.Done()
		for {
			select {
			case event := <-manager.onNewBlock:
				if err := manager.blockchain.ProcessNewBlock(event.SerializedBlock, false); err != nil {
					utils.Tracef("[P2P %s] AddBlockSerialized %d failed %v", event.source.logPrefix, event.SerializedBlock.Header.Index, err)
				} else if event.shouldBroadcast {
					manager.broadcastBlock(&event.SerializedBlock, event.source)
				}
			case event := <-manager.onNewOperation:
				new, err := manager.blockchain.TxPoolAddOperation(event.CommonOperation, false)
				if err != nil {
					utils.Tracef("[P2P %s] Tx validation failed: %v", event.source.logPrefix, err)
				} else if new {
					manager.broadcastTx(event.CommonOperation, event.source)
				}
			case conn := <-manager.closed:
				manager.initializedConnections.Delete(conn)
			case event := <-manager.onStateUpdate:
				manager.initializedConnections.Store(event.source, event.topBlockIndex)
				if event.source.onStateUpdated != nil {
					event.source.onStateUpdated()
				}
				signalSync()
			case state := <-manager.onSyncState:
				if state != manager.prevSyncState {
					manager.prevSyncState = state
					switch manager.prevSyncState {
					case synced:
						utils.Tracef("Synchronized with the network at height %d", manager.blockchain.GetHeight())
					case syncing:
						utils.Tracef("Synchronizing with the network")
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	manager.waitGroup.Add(1)
	go func() {
		defer manager.waitGroup.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				if !manager.sync(ctx) {
					manager.doSync.L.Lock()
					if !manager.doSyncValue {
						manager.doSync.Wait()
						manager.doSyncValue = false
					}
					manager.doSync.L.Unlock()
				}
			}
		}
	}()

	return callback(manager)
}

func (this *Manager) sync(ctx context.Context) bool {
	result := false

	nodeHeight := this.blockchain.GetHeight()
	for {
		select {
		case <-ctx.Done():
			return result
		default:
			break
		}

		candidates := make([]*PascalConnection, 0)
		connections := 0
		this.initializedConnections.Range(func(conn, topBlockIndex interface{}) bool {
			connections++
			if topBlockIndex.(uint32) >= nodeHeight {
				candidates = append(candidates, conn.(*PascalConnection))
			}
			return true
		})

		candidatesTotal := len(candidates)
		if candidatesTotal == 0 {
			if connections > 0 {
				this.onSyncState <- synced
			}
			break
		} else {
			this.onSyncState <- syncing
		}

		selected := rand.Int() % candidatesTotal
		conn := candidates[selected]

		topBlockIndex, ok := this.initializedConnections.Load(conn)
		if !ok {
			continue
		}

		to := utils.MinUint32(nodeHeight+defaults.NetworkBlocksPerRequest-1, topBlockIndex.(uint32))
		ahead := topBlockIndex.(uint32) + 1 - nodeHeight
		utils.Tracef("[P2P %s] Fetching blocks %d .. %d (%d blocks ~%d days ahead)", conn.logPrefix, nodeHeight, to, ahead, ahead/288)

		blocks := conn.BlocksGet(nodeHeight, to)
		switch err := this.blockchain.ProcessNewBlocks(blocks, nil); err {
		case nil:
			{
				nodeHeight += uint32(len(blocks))
			}
		case blockchain.ErrParentNotFound:
			{
				from := utils.MaxUint32(blocks[0].Header.Index, defaults.MaxAltChainLength) - defaults.MaxAltChainLength
				to = utils.MaxUint32(nodeHeight, 1) - 1
				utils.Tracef("[P2P %s] Fetching alternate chain, downloading blocks %d .. %d", conn.logPrefix, from, to)
				blocks = append(conn.BlocksGet(from, to), blocks...)

				utils.Tracef("[P2P %s] Processing alternate chain, downloaded %d blocks", conn.logPrefix, len(blocks))
				if err := this.blockchain.AddAlternateChain(blocks); err != nil {
					utils.Tracef("[P2P %s] Failed to switch to alternate chain: %v", conn.logPrefix, err)
					return false
				}
				utils.Tracef("[P2P %s] Switched to alternate chain", conn.logPrefix)
			}
		default:
			{
				utils.Tracef("[P2P %s] Verification failed %v", conn.logPrefix, err)
				return false
			}
		}

		result = true
	}

	return result
}

func (this *Manager) forEachConnection(fn func(*PascalConnection), except *PascalConnection) {
	this.initializedConnections.Range(func(conn, height interface{}) bool {
		if conn != except {
			fn(conn.(*PascalConnection))
		}
		return true
	})
}

func (m *Manager) broadcastBlock(block *safebox.SerializedBlock, except *PascalConnection) {
	m.forEachConnection(func(conn *PascalConnection) {
		conn.BroadcastBlock(block)
	}, except)
}

func (m *Manager) broadcastTx(transaction tx.CommonOperation, except *PascalConnection) {
	m.forEachConnection(func(conn *PascalConnection) {
		conn.BroadcastTx(transaction)
	}, except)
}

func (m *Manager) OnNewConnection(ctx context.Context, c *network.Connection) error {
	stopper := concurrent.NewUnboundedExecutor()
	stopper.Go(func(localContext context.Context) {
		select {
		case <-ctx.Done():
			break
		case <-localContext.Done():
			break
		}
		c.Transport.Close()
	})
	defer stopper.StopAndWaitForever()

	link, err := m.OnOpen(
		c.Address,
		c.Transport,
		c.Outgoing,
		c.OnStateUpdated,
		func(p *PascalConnection) error {
			if p.outgoing && bytes.Equal(m.nonce, p.GetRemoteNonce()) {
				return network.ErrLoopbackConnection
			}

			err := error(nil)
			m.forEachConnection(func(conn *PascalConnection) {
				if bytes.Equal(conn.GetRemoteNonce(), p.GetRemoteNonce()) {
					err = network.ErrDuplicateConnection
				}
			}, p)
			return err
		},
	)
	if err != nil {
		utils.Tracef("OnOpen failed: %v", err)
		return err
	}
	defer m.OnClose(link)

	buf := make([]byte, 10*1024)
	for {
		read, err := c.Transport.Read(buf)
		if err != nil {
			return err
		}
		if err = m.OnData(link, buf[:read]); err != nil {
			utils.Tracef("OnData failed: %v", err)
			return err
		}
	}
}

func (this *Manager) OnOpen(
	address string,
	transport io.WriteCloser,
	isOutgoing bool,
	onStateUpdated func(),
	postHandshake func(*PascalConnection) error,
) (interface{}, error) {
	conn := &PascalConnection{
		underlying:     NewProtocol(transport, this.timeoutRequest),
		logPrefix:      address,
		blockchain:     this.blockchain,
		p2pPort:        this.p2pPort,
		peers:          this.peers,
		nonce:          this.nonce,
		peerUpdates:    this.peerUpdates,
		onStateUpdate:  this.onStateUpdate,
		onNewOperation: this.onNewOperation,
		closed:         this.closed,
		onNewBlock:     this.onNewBlock,
		onStateUpdated: onStateUpdated,
		postHandshake:  postHandshake,
		outgoing:       isOutgoing,
	}

	if err := conn.OnOpen(); err != nil {
		return nil, err
	}

	this.waitGroup.Add(1)
	return conn, nil
}

func (this *Manager) OnData(connection interface{}, data []byte) error {
	return connection.(*PascalConnection).OnData(data)
}

func (this *Manager) OnClose(connection interface{}) {
	connection.(*PascalConnection).OnClose()
	this.waitGroup.Done()
}

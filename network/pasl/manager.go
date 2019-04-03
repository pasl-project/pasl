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
	"context"
	"io"
	"math/rand"
	"sync"
	"time"

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

type syncState int

const (
	synced  syncState = iota
	syncing           = iota
)

type manager struct {
	network.Manager

	waitGroup sync.WaitGroup

	blockchain *blockchain.Blockchain
	nonce      []byte

	timeoutRequest         time.Duration
	blocksUpdates          <-chan safebox.SerializedBlock
	peerUpdates            chan<- PeerInfo
	txPoolUpdates          <-chan tx.CommonOperation
	onStateUpdate          chan *PascalConnection
	onNewBlock             chan *eventNewBlock
	onNewOperation         chan *eventNewOperation
	onSyncState            chan syncState
	prevSyncState          syncState
	closed                 chan *PascalConnection
	initializedConnections sync.Map
	doSyncValue            bool
	doSync                 *sync.Cond
}

func WithManager(nonce []byte, blockchain *blockchain.Blockchain, peerUpdates chan<- PeerInfo, blocksUpdates <-chan safebox.SerializedBlock, txPoolUpdates <-chan tx.CommonOperation, timeoutRequest time.Duration, callback func(network.Manager) error) error {
	manager := &manager{
		timeoutRequest: timeoutRequest,
		blockchain:     blockchain,
		nonce:          nonce,
		blocksUpdates:  blocksUpdates,
		peerUpdates:    peerUpdates,
		txPoolUpdates:  txPoolUpdates,
		onStateUpdate:  make(chan *PascalConnection),
		onNewOperation: make(chan *eventNewOperation),
		onSyncState:    make(chan syncState),
		prevSyncState:  syncing,
		closed:         make(chan *PascalConnection),
		onNewBlock:     make(chan *eventNewBlock),
		doSync:         sync.NewCond(&sync.Mutex{}),
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
			case conn := <-manager.onStateUpdate:
				topBlockIndex, _ := conn.GetState()
				manager.initializedConnections.Store(conn, topBlockIndex)
				if conn.onStateUpdated != nil {
					conn.onStateUpdated()
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
			case block := <-manager.blocksUpdates:
				utils.Tracef("Broadcasting tx")
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

func (this *manager) sync(ctx context.Context) bool {
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
		topBlockIndex, _ := conn.GetState()
		to := utils.MinUint32(nodeHeight+defaults.NetworkBlocksPerRequest-1, topBlockIndex)
		ahead := topBlockIndex + 1 - nodeHeight
		utils.Tracef("[P2P %s] Fetching blocks %d .. %d (%d blocks ~%d days ahead)", conn.logPrefix, nodeHeight, to, ahead, ahead/288)

		blocks := conn.BlocksGet(nodeHeight, to)
		nodeHeight += uint32(len(blocks))

		switch err := this.blockchain.ProcessNewBlocks(blocks, nil); err {
		case nil:
			{
				continue
			}
		case blockchain.ErrParentNotFound:
			{
				utils.Tracef("[P2P %s] Fetching alternate chain", conn.logPrefix)

				from := utils.MaxUint32(blocks[0].Header.Index, defaults.MaxAltChainLength) - defaults.MaxAltChainLength
				blocks := conn.BlocksGet(from, to)

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

func (this *manager) forEachConnection(fn func(*PascalConnection), except *PascalConnection) {
	this.initializedConnections.Range(func(conn, height interface{}) bool {
		if conn != except {
			fn(conn.(*PascalConnection))
		}
		return true
	})
}

func (m *manager) broadcastBlock(block *safebox.SerializedBlock, except *PascalConnection) {
	m.forEachConnection(func(conn *PascalConnection) {
		conn.BroadcastBlock(block)
	}, except)
}

func (m *manager) broadcastTx(transaction tx.CommonOperation, except *PascalConnection) {
	m.forEachConnection(func(conn *PascalConnection) {
		conn.BroadcastTx(transaction)
	}, except)
}

func (this *manager) OnOpen(address string, transport io.WriteCloser, isOutgoing bool, onStateUpdated func()) (interface{}, error) {
	conn := &PascalConnection{
		underlying:     NewProtocol(transport, this.timeoutRequest),
		logPrefix:      address,
		blockchain:     this.blockchain,
		nonce:          this.nonce,
		peerUpdates:    this.peerUpdates,
		onStateUpdate:  this.onStateUpdate,
		onNewOperation: this.onNewOperation,
		closed:         this.closed,
		onNewBlock:     this.onNewBlock,
		onStateUpdated: onStateUpdated,
	}

	if err := conn.OnOpen(isOutgoing); err != nil {
		return nil, err
	}

	this.waitGroup.Add(1)
	return conn, nil
}

func (this *manager) OnData(connection interface{}, data []byte) error {
	return connection.(*PascalConnection).OnData(data)
}

func (this *manager) OnClose(connection interface{}) {
	connection.(*PascalConnection).OnClose()
	this.waitGroup.Done()
}

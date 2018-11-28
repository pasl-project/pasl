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
	tx.Tx
}

type manager struct {
	network.Manager

	waitGroup sync.WaitGroup

	blockchain *blockchain.Blockchain
	nonce      []byte

	timeoutRequest         time.Duration
	peerUpdates            chan<- PeerInfo
	onStateUpdate          chan *PascalConnection
	onNewBlock             chan *eventNewBlock
	onNewOperation         chan *eventNewOperation
	closed                 chan *PascalConnection
	initializedConnections sync.Map
	doSyncValue            bool
	doSync                 *sync.Cond
}

func WithManager(nonce []byte, blockchain *blockchain.Blockchain, peerUpdates chan<- PeerInfo, timeoutRequest time.Duration, callback func(network.Manager) error) error {
	manager := &manager{
		timeoutRequest: timeoutRequest,
		blockchain:     blockchain,
		nonce:          nonce,
		peerUpdates:    peerUpdates,
		onStateUpdate:  make(chan *PascalConnection),
		onNewOperation: make(chan *eventNewOperation),
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
				if _, _, err := manager.blockchain.AddBlockSerialized(&event.SerializedBlock, false); err != nil {
					utils.Tracef("[P2P %s] AddBlockSerialized %d failed %v", event.source.logPrefix, event.SerializedBlock.Header.Index, err)
				} else if event.shouldBroadcast {
					manager.forEachConnection(func(conn *PascalConnection) {
						conn.BroadcastBlock(&event.SerializedBlock)
					}, event.source)
				}
			case event := <-manager.onNewOperation:
				new, err := manager.blockchain.TxPoolAddOperation(&event.Tx)
				if err != nil {
					utils.Tracef("[P2P %s] Tx validation failed: %v", event.source.logPrefix, err)
				} else if new {
					manager.forEachConnection(func(conn *PascalConnection) {
						conn.BroadcastTx(&event.Tx)
					}, event.source)
				}
			case conn := <-manager.closed:
				manager.initializedConnections.Delete(conn)
			case conn := <-manager.onStateUpdate:
				connHeight, _ := conn.GetState()
				manager.initializedConnections.Store(conn, connHeight)
				if conn.onStateUpdated != nil {
					conn.onStateUpdated()
				}
				signalSync()
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

	nodeHeight, _, _ := this.blockchain.GetState()
	for {
		select {
		case <-ctx.Done():
			return result
		default:
			break
		}

		candidates := make([]*PascalConnection, 0)
		this.initializedConnections.Range(func(conn, height interface{}) bool {
			if height.(uint32) > nodeHeight {
				candidates = append(candidates, conn.(*PascalConnection))
			}
			return true
		})

		candidatesTotal := len(candidates)
		if candidatesTotal == 0 {
			return result
		}

		selected := rand.Int() % candidatesTotal
		conn := candidates[selected]
		height, _ := conn.GetState()
		ahead := height - nodeHeight
		utils.Tracef("[P2P %s] Fetching blocks %d -> %d (%d blocks ~%d days ahead)", conn.logPrefix, nodeHeight, height, ahead, ahead/288)

		to := utils.MinUint32(nodeHeight+defaults.NetworkBlocksPerRequest-1, height-1)
		blocks := conn.BlocksGet(nodeHeight, to)
		nodeHeight += uint32(len(blocks))

		if added, block, err := this.blockchain.AddBlocksSerialized(blocks); err != nil {
					utils.Tracef("[P2P %s] Block #%d verification failed %v", conn.logPrefix, block.Header.Index, err)
			return added > 0
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

/*
PASL - Personalized Accounts & Secure Ledger

Copyright (C) 2018 PASL Project

Greatly inspired by Kurt Rose's python implementation
https://gist.github.com/kurtbrose/4423605

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

	blockchain  		   *blockchain.Blockchain
	nonce				   []byte

	timeoutRequest         time.Duration
	peerUpdates            chan<- PeerInfo
	onStateUpdate          chan *PascalConnection
	onNewBlock             chan *eventNewBlock
	onNewOperation         chan *eventNewOperation
	closed                 chan *PascalConnection
	initializedConnections map[*PascalConnection]uint32
	downloading            bool
	downloadingDone        chan interface{}
}

func WithManager(nonce []byte, blockchain *blockchain.Blockchain, peerUpdates chan<- PeerInfo, timeoutRequest time.Duration, callback func(network.Manager) error) error {
	manager := &manager{
		timeoutRequest:         timeoutRequest,
		blockchain:             blockchain,
		nonce:                  nonce,
		peerUpdates:            peerUpdates,
		onStateUpdate:          make(chan *PascalConnection),
		onNewOperation:         make(chan *eventNewOperation),
		closed:                 make(chan *PascalConnection),
		onNewBlock:             make(chan *eventNewBlock),
		initializedConnections: make(map[*PascalConnection]uint32),
		downloading:            false,
		downloadingDone:        make(chan interface{}),
	}
	defer manager.waitGroup.Wait()

	stop := make(chan bool)
	go func() {
		defer manager.waitGroup.Done()

		for {
			select {
			case event := <-manager.onNewBlock:
				if err := manager.blockchain.AddBlockSerialized(&event.SerializedBlock); err != nil {
					utils.Tracef("[P2P] AddBlockSerialized %d failed %v", event.SerializedBlock.Header.Index, err)
					if event.shouldBroadcast {
						manager.forEachConnection(func(conn *PascalConnection) {
							conn.BroadcastBlock(&event.SerializedBlock)
						}, event.source)
					}
				}
			case event := <-manager.onNewOperation:
				new, err := manager.blockchain.AddOperation(&event.Tx)
				if err != nil {
					utils.Tracef("[P2P %p] Tx validation failed: %v", event.source, err)
				} else if new {
					manager.forEachConnection(func(conn *PascalConnection) {
						conn.BroadcastTx(&event.Tx)
					}, event.source)
				}
			case <-manager.downloadingDone:
				manager.downloading = false
				manager.startDownloading()
			case conn := <-manager.closed:
				delete(manager.initializedConnections, conn)
			case conn := <-manager.onStateUpdate:
				connHeight, _ := conn.GetState()
				manager.initializedConnections[conn] = connHeight
				manager.startDownloading()
			case <-stop:
			}
		}
	}()

	err := callback(manager)
	stop <- true

	return err
}

func (this *manager) startDownloading() {
	if this.downloading {
		return
	}

	nodeHeight, _ := this.blockchain.GetState()

	candidates := make(map[uint32]*PascalConnection)
	for conn, height := range this.initializedConnections {
		if height > nodeHeight {
			candidates[rand.Uint32()] = conn
		}
	}

	for _, conn := range candidates {
		height, _ := conn.GetState()
		utils.Tracef("[P2P %p] Remote node height %d (%d blocks ahead)", conn, height, height-nodeHeight)

		this.downloading = true
		to := utils.MinUint32(nodeHeight+defaults.NetworkBlocksPerRequest-1, height-1)
		if err := conn.StartBlocksDownloading(nodeHeight, to, this.downloadingDone); err == nil {
			utils.Tracef("[P2P %p] Downloading blocks #%d .. #%d", conn, nodeHeight, to)
			break
		} else {
			this.downloading = false
		}
	}

	if !this.downloading && len(candidates) == 0 {
		utils.Tracef("On main chain, height %d", nodeHeight)
	}
}

func (this *manager) forEachConnection(fn func(*PascalConnection), except *PascalConnection) {
	for conn := range this.initializedConnections {
		if conn != except {
			fn(conn)
		}
	}
}

func (this *manager) OnOpen(address string, transport io.WriteCloser, isOutgoing bool) (interface{}, error) {
	conn := &PascalConnection{
		underlying:     NewProtocol(transport, this.timeoutRequest),
		blockchain:     this.blockchain,
		nonce:          this.nonce,
		peerUpdates:    this.peerUpdates,
		onStateUpdate:  this.onStateUpdate,
		onNewOperation: this.onNewOperation,
		closed:         this.closed,
		onNewBlock:     this.onNewBlock,
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

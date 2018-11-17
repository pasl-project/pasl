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
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/pasl-project/pasl/blockchain"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/utils"
)

type pascalConnectionState struct {
	height          uint32
	prevSafeboxHash []byte
}

type PascalConnection struct {
	logPrefix      string
	underlying     *protocol
	blockchain     *blockchain.Blockchain
	nonce          []byte
	peerUpdates    chan<- PeerInfo
	onStateUpdate  chan<- *PascalConnection
	onNewBlock     chan *eventNewBlock
	onNewOperation chan<- *eventNewOperation
	state          *pascalConnectionState
	stateLock      sync.RWMutex
	closed         chan *PascalConnection
	onStateUpdated func()
}

func (this *PascalConnection) OnOpen(isOutgoing bool) error {
	this.underlying.knownOperations[hello] = this.onHelloRequest
	this.underlying.knownOperations[errorReport] = this.onErrorReport
	this.underlying.knownOperations[message] = this.onMessageRequest
	this.underlying.knownOperations[getBlocks] = this.onGetBlocksRequest
	this.underlying.knownOperations[getHeaders] = this.onGetHeadersRequest
	this.underlying.knownOperations[newBlock] = this.onNewBlockNotification
	this.underlying.knownOperations[newOperations] = this.onNewOperationsNotification

	if !isOutgoing {
		return nil
	}

	payload := generateHello(0, this.nonce, this.blockchain.SerializeBlockHeader(this.blockchain.GetPendingBlock(nil), false, false), nil, defaults.UserAgent)
	return this.underlying.sendRequest(hello, payload, this.onHelloCommon)
}

func (this *PascalConnection) OnData(data []byte) error {
	return this.underlying.OnData(data)
}

func (this *PascalConnection) OnClose() {
	this.closed <- this
}

func (this *PascalConnection) SetState(height uint32, prevSafeboxHash []byte) {
	this.stateLock.Lock()
	defer this.stateLock.Unlock()
	defer func() { this.onStateUpdate <- this }()

	state := &pascalConnectionState{
		height:          height,
		prevSafeboxHash: make([]byte, 32),
	}
	copy(state.prevSafeboxHash[:32], prevSafeboxHash)
	this.state = state
}

func (this *PascalConnection) GetState() (uint32, []byte) {
	this.stateLock.RLock()
	defer this.stateLock.RUnlock()
	return this.state.height, this.state.prevSafeboxHash
}

func (this *PascalConnection) BlocksGet(from, to uint32) []safebox.SerializedBlock {
	packet := utils.Serialize(packetGetBlocksRequest{
		FromIndex: from,
		ToIndex:   to,
	})

	blocks := make([]safebox.SerializedBlock, 0)

	finished := sync.WaitGroup{}
	finished.Add(1)
	err := this.underlying.sendRequest(getBlocks, packet, func(response *requestResponse, payload []byte) error {
		defer finished.Done()

		if response == nil {
			return errors.New("GetBlocks request failed")
		}

		var packet packetGetBlocksResponse
		if err := utils.Deserialize(&packet, bytes.NewBuffer(payload)); err != nil {
			return err
		}

		blocks = append(blocks, packet.Blocks...)
		return nil
	})
	if err == nil {
		finished.Wait()
	}

	return blocks
}

func (this *PascalConnection) BroadcastTx(operation *tx.Tx) {
	packet := packetNewOperations{
		OperationsNetwork: tx.OperationsNetwork{
			Operations: []tx.Tx{*operation},
		},
	}
	this.underlying.sendRequest(newOperations, utils.Serialize(&packet), nil)
}

func (this *PascalConnection) BroadcastBlock(block *safebox.SerializedBlock) {
	this.underlying.sendRequest(newBlock, utils.Serialize(packetNewBlock{*block}), nil)
}

func (this *PascalConnection) onHelloCommon(request *requestResponse, payload []byte) error {
	utils.Tracef("[P2P %s] %s", this.logPrefix, request.GetType())

	if request == nil {
		return fmt.Errorf("[P2P %s] Refused by remote side", this.logPrefix)
	}

	var packet packetHello
	if err := utils.Deserialize(&packet, bytes.NewBuffer(payload)); err != nil {
		request.result.setError(invalidDataBufferInfo)
		return err
	}

	if bytes.Equal(packet.Nonce, this.nonce) {
		return fmt.Errorf("[P2P %s] Loopback connection", this.logPrefix)
	}

	utils.Tracef("[P2P %s] Height %d SafeboxHash %s", this.logPrefix, packet.Block.Index, hex.EncodeToString(packet.Block.PrevSafeboxHash))
	this.SetState(packet.Block.Index, packet.Block.PrevSafeboxHash)

	for _, peer := range packet.Peers {
		this.peerUpdates <- peer
	}

	return nil
}

func (this *PascalConnection) onHelloRequest(request *requestResponse, payload []byte) ([]byte, error) {
	if err := this.onHelloCommon(request, payload); err != nil {
		return nil, err
	}

	out := generateHello(0, this.nonce, this.blockchain.SerializeBlockHeader(this.blockchain.GetPendingBlock(nil), false, false), nil, defaults.UserAgent)
	request.result.setError(success)
	return out, nil
}

func (this *PascalConnection) onGetBlocksRequest(request *requestResponse, payload []byte) ([]byte, error) {
	utils.Tracef("[P2P %s] %s", this.logPrefix, request.GetType())

	var packet packetGetBlocksRequest
	if err := utils.Deserialize(&packet, bytes.NewBuffer(payload)); err != nil {
		return nil, err
	}

	if packet.FromIndex > packet.ToIndex {
		packet.ToIndex, packet.FromIndex = packet.FromIndex, packet.ToIndex
	}

	total := packet.ToIndex - packet.FromIndex
	if total > defaults.NetworkBlocksPerRequest {
		total = defaults.NetworkBlocksPerRequest
		packet.ToIndex = packet.FromIndex + total
	}

	serialized := make([]safebox.SerializedBlock, total)
	for index := packet.FromIndex; index <= packet.ToIndex; index++ {
		if block := this.blockchain.GetBlock(index); block != nil {
			serialized = append(serialized, this.blockchain.SerializeBlock(block))
		} else {
			utils.Tracef("[P2P %s] Failed to get block %d", this.logPrefix, index)
			break
		}
	}

	out := utils.Serialize(packetGetBlocksResponse{
		Blocks: serialized,
	})
	request.result.setError(success)

	return out, nil
}

func (this *PascalConnection) onErrorReport(request *requestResponse, payload []byte) ([]byte, error) {
	var packet packetError
	if err := utils.Deserialize(&packet, bytes.NewBuffer(payload)); err != nil {
		return nil, err
	}

	utils.Tracef("[P2P %s] Peer reported error '%s'", this.logPrefix, packet.Message)

	return nil, nil
}

func (this *PascalConnection) onMessageRequest(request *requestResponse, payload []byte) ([]byte, error) {
	utils.Tracef("[P2P %s] %s", this.logPrefix, request.GetType())
	return nil, nil
}

func (this *PascalConnection) onGetHeadersRequest(request *requestResponse, payload []byte) ([]byte, error) {
	utils.Tracef("[P2P %s] %s", this.logPrefix, request.GetType())
	return nil, nil
}

func (this *PascalConnection) onNewBlockNotification(request *requestResponse, payload []byte) ([]byte, error) {
	var packet packetNewBlock
	if err := utils.Deserialize(&packet, bytes.NewBuffer(payload)); err != nil {
		return nil, err
	}

	utils.Tracef("[P2P %s] New block %d", this.logPrefix, packet.Header.Index)
	this.onNewBlock <- &eventNewBlock{
		event:           event{this},
		SerializedBlock: packet.SerializedBlock,
		shouldBroadcast: true,
	}

	return nil, nil
}

func (this *PascalConnection) onNewOperationsNotification(request *requestResponse, payload []byte) ([]byte, error) {
	var packet packetNewOperations
	if err := utils.Deserialize(&packet, bytes.NewBuffer(payload)); err != nil {
		return nil, err
	}

	utils.Tracef("[P2P %s] New operations %d", this.logPrefix, len(packet.Operations))
	for _, op := range packet.Operations {
		this.onNewOperation <- &eventNewOperation{event{this}, op}
	}

	return nil, nil
}

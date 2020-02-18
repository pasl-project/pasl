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
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modern-go/concurrent"
	"github.com/pasl-project/pasl/blockchain"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/network"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/utils"
)

type PascalConnection struct {
	logPrefix      string
	underlying     *protocol
	blockchain     *blockchain.Blockchain
	p2pPort        uint16
	peers          *network.PeersList
	nonce          []byte
	remoteNonce    []byte
	peerUpdates    chan<- network.PeerInfo
	onStateUpdate  chan<- eventConnectionState
	onNewBlock     chan *eventNewBlock
	onNewOperation chan<- *eventNewOperation
	closed         chan *PascalConnection
	onStateUpdated func()
	postHandshake  func(*PascalConnection) error
	handshakeDone  uint32
	outgoing       bool
	periodic       *concurrent.UnboundedExecutor
}

func (p *PascalConnection) OnOpen() error {
	p.underlying.knownOperations[hello] = p.onHelloRequest
	p.underlying.knownOperations[errorReport] = p.onErrorReport
	p.underlying.knownOperations[message] = p.onMessageRequest
	p.underlying.knownOperations[getBlocks] = p.onGetBlocksRequest
	p.underlying.knownOperations[getHeaders] = p.onGetHeadersRequest
	p.underlying.knownOperations[newBlock] = p.onNewBlockNotification
	p.underlying.knownOperations[newOperations] = p.onNewOperationsNotification
	p.periodic = concurrent.NewUnboundedExecutor()

	if p.outgoing {
		p.periodic.Go(p.PeriodicPing)
	}

	return nil
}

func (this *PascalConnection) OnData(data []byte) error {
	return this.underlying.OnData(data)
}

func (p *PascalConnection) OnClose() {
	p.periodic.StopAndWaitForever()
	p.closed <- p
}

func (p *PascalConnection) PeriodicPing(ctx context.Context) {
	interval := time.Duration(30) * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if topBlock, err := p.blockchain.GetTopBlock(); err == nil {
			payload := generateHello(p.p2pPort, p.nonce, p.blockchain.SerializeBlockHeader(topBlock, false, false), p.peers.GetAllSeen(), defaults.UserAgent)
			p.underlying.sendRequest(hello, payload, p.onHelloCommon)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			break
		}
	}
}

func (p *PascalConnection) GetRemoteNonce() []byte {
	return p.remoteNonce
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

	sort.Slice(blocks, func(i, j int) bool { return blocks[i].Header.Index < blocks[j].Header.Index })
	return blocks
}

func (this *PascalConnection) BroadcastTx(operation tx.CommonOperation) {
	packet := packetNewOperations{
		OperationsNetwork: tx.OperationsNetwork{
			Operations: []tx.CommonOperation{operation},
		},
	}
	this.underlying.sendRequest(newOperations, utils.Serialize(&packet), nil)
}

func (this *PascalConnection) BroadcastBlock(block *safebox.SerializedBlock) {
	this.underlying.sendRequest(newBlock, utils.Serialize(packetNewBlock{*block}), nil)
}

func (this *PascalConnection) onHelloCommon(request *requestResponse, payload []byte) error {
	if request == nil {
		return fmt.Errorf("[P2P %s] Refused by remote side", this.logPrefix)
	}

	var packet packetHello
	if err := utils.Deserialize(&packet, bytes.NewBuffer(payload)); err != nil {
		request.result.setError(invalidDataBufferInfo)
		return err
	}

	utils.Tracef("[P2P %s] Top block %d SafeboxHash %s", this.logPrefix, packet.Block.Index, hex.EncodeToString(packet.Block.PrevSafeboxHash))
	this.onStateUpdate <- eventConnectionState{event{this}, packet.Block.Index}

	if atomic.CompareAndSwapUint32(&this.handshakeDone, 0, 1) {
		this.remoteNonce = packet.Nonce
		if err := this.postHandshake(this); err != nil {
			return err
		}
	}

	for _, peer := range packet.Peers {
		this.peerUpdates <- peer
	}

	return nil
}

func (this *PascalConnection) onHelloRequest(request *requestResponse, payload []byte) ([]byte, error) {
	if err := this.onHelloCommon(request, payload); err != nil {
		return nil, err
	}

	topBlock, err := this.blockchain.GetTopBlock()
	if err != nil {
		return nil, err
	}

	out := generateHello(this.p2pPort, this.nonce, this.blockchain.SerializeBlockHeader(topBlock, false, false), nil, defaults.UserAgent)
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

	serialized := make([]safebox.SerializedBlock, 0, total)
	for index := packet.FromIndex; index <= packet.ToIndex; index++ {
		if block, err := this.blockchain.GetBlock(index); err == nil {
			serialized = append(serialized, this.blockchain.SerializeBlock(block))
		} else {
			utils.Tracef("[P2P %s] Failed to get block %d: %v", this.logPrefix, index, err)
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

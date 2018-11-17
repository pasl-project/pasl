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
	"encoding/binary"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/modern-go/concurrent"
	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/defaults"
)

const headerSize = 4 + 2 + 2 + 2 + 4 + 2 + 2 + 4

type typeId int16

const (
	request      typeId = 1
	response     typeId = 2
	notification typeId = 3
)

type operationId int16

const (
	_ operationId = iota
	hello
	errorReport
	message
	getBlocks     = 0x10
	getHeaders    = 0x5
	newBlock      = 0x11
	newOperations = 0x20
)

type errorId int16

const (
	success errorId = iota
	invalidProtocolVersion
	ipBlackListed
	invalidDataBufferInfo = 0x0010
	internalServerError   = 0x0011
	invalidNewAccount     = 0x0012
)

type packetHeader struct {
	NetworkId   uint32
	TypeId      typeId
	Operation   operationId
	Error       errorId
	RequestId   uint32
	Version     common.Version
	PayloadSize uint32
}

type result struct {
	errorId errorId
}

func (this *result) getError() errorId {
	return this.errorId
}

func (this *result) setError(errorId errorId) {
	this.errorId = errorId
}

type requestResponse struct {
	id        uint32
	typeId    typeId
	operation operationId
	expecting int
	result    *result
}

func (r *requestResponse) GetType() string {
	if r == nil {
		return "cancelled"
	}
	switch r.typeId {
	case request:
		return "request"
	case response:
		return "response"
	case notification:
		return "notification"
	default:
		return "unknown"
	}
}

type requestHandler func(request *requestResponse, payload []byte) ([]byte, error)
type responseHandler func(request *requestResponse, payload []byte) error

type requestWithTimeout struct {
	responseHandler
	*concurrent.UnboundedExecutor
}

func NewRequest(handler responseHandler, onTimeout func(), timeoutRequest time.Duration) *requestWithTimeout {
	unboundedExecutor := concurrent.NewUnboundedExecutor()
	unboundedExecutor.Go(func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(timeoutRequest):
			onTimeout()
			return
		}
	})

	return &requestWithTimeout{handler, unboundedExecutor}
}

func (this *requestWithTimeout) Process(packet *requestResponse, payload []byte) error {
	this.UnboundedExecutor.StopAndWaitForever()
	return this.responseHandler(packet, payload)
}

type protocol struct {
	transport       io.WriteCloser
	timeoutRequest  time.Duration
	requests        map[uint32]*requestWithTimeout
	requestId       uint32
	buffer          *bytes.Buffer
	header          packetHeader
	pendingPacket   *requestResponse
	knownOperations map[operationId]requestHandler
}

func NewProtocol(transport io.WriteCloser, timeoutRequest time.Duration) *protocol {
	conn := &protocol{
		transport:       transport,
		timeoutRequest:  timeoutRequest,
		buffer:          &bytes.Buffer{},
		knownOperations: make(map[operationId]requestHandler),
		requests:        make(map[uint32]*requestWithTimeout),
	}
	return conn
}

func (this *protocol) Close() error {
	for _, request := range this.requests {
		request.Process(nil, nil)
	}

	return this.transport.Close()
}

func (this *protocol) OnData(data []byte) error {
	err := binary.Write(this.buffer, binary.LittleEndian, data)
	if err != nil {
		return err
	}

	for {
		if this.pendingPacket == nil {
			if this.buffer.Len() < headerSize {
				break
			}
			this.pendingPacket, err = this.parseHeader(this.buffer.Next(headerSize))
		} else {
			if this.buffer.Len() < this.pendingPacket.expecting {
				break
			}

			payloadIn := this.buffer.Next(this.pendingPacket.expecting)
			err = this.onPacket(this.pendingPacket, payloadIn)
			this.pendingPacket = nil

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (this *protocol) onPacket(packet *requestResponse, payload []byte) (err error) {
	var out []byte = nil

	out, err = this.processPacket(packet, payload)
	if err != nil {
		return err
	}

	if packet.typeId == request {
		return this.sendResponse(packet, out)
	}

	return nil
}

func (this *protocol) processPacket(packet *requestResponse, payload []byte) (out []byte, err error) {
	if packet.typeId == response {
		if request, ok := this.requests[packet.id]; ok {
			delete(this.requests, packet.id)
			return nil, request.Process(packet, payload)
		}
		return nil, errors.New("Unexpected response")
	}

	if handler, ok := this.knownOperations[packet.operation]; ok {
		return handler(packet, payload)
	}

	packet.result.setError(success)
	return nil, nil
}

func (this *protocol) sendRequest(operationId operationId, payload []byte, handler responseHandler) error {
	newRequestId := atomic.AddUint32(&this.requestId, 1)

	var packetType typeId
	if handler != nil {
		packetType = typeId(request)
	} else {
		packetType = typeId(notification)
	}

	packet, err := this.preparePacket(packetType, operationId, newRequestId, success, payload)
	if err != nil {
		return err
	}

	if handler != nil {
		this.requests[newRequestId] = NewRequest(handler, func() {
			delete(this.requests, newRequestId)
			if err := handler(nil, nil); err != nil {
				this.Close()
			}
		}, this.timeoutRequest)
	}

	_, err = this.transport.Write(packet)
	return err
}

func (this *protocol) sendResponse(request *requestResponse, payload []byte) error {
	packet, err := this.preparePacket(typeId(response), request.operation, request.id, request.result.getError(), payload)
	if err != nil {
		return err
	}
	_, err = this.transport.Write(packet)
	return err
}

func (this *protocol) preparePacket(typeId typeId, operationId operationId, requestId uint32, errorId errorId, payload []byte) (data []byte, err error) {
	packet := &bytes.Buffer{}
	err = binary.Write(packet, binary.LittleEndian, &packetHeader{
		NetworkId: defaults.NetId,
		TypeId:    typeId,
		Operation: operationId,
		Error:     errorId,
		RequestId: requestId,
		Version: common.Version{
			Major: 3,
			Minor: 4,
		},
		PayloadSize: uint32(len(payload)),
	})
	if err != nil {
		return
	}
	if len(payload) > 0 {
		var n int
		n, err = packet.Write(payload)
		if err != nil {
			return
		}
		if n != len(payload) {
			return nil, errors.New("Memory allocation error")
		}
	}
	return packet.Bytes(), nil
}

func (this *protocol) parseHeader(data []byte) (handleWith *requestResponse, err error) {
	err = binary.Read(bytes.NewReader(data), binary.LittleEndian, &this.header)
	if err != nil {
		return
	}

	if this.header.NetworkId != defaults.NetId {
		err = errors.New("Invalid network id")
		return
	}

	return &requestResponse{
		id:        this.header.RequestId,
		typeId:    this.header.TypeId,
		operation: this.header.Operation,
		expecting: int(this.header.PayloadSize),
		result:    &result{},
	}, nil
}

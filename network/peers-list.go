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
	"sync"
	"time"

	"github.com/cevaris/ordered_map"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

type PeersList struct {
	Connected map[string]*Peer
	Queued    *ordered_map.OrderedMap
	Lock      sync.RWMutex
}

func NewPeersList() *PeersList {
	return &PeersList{
		Connected: make(map[string]*Peer),
		Queued:    ordered_map.NewOrderedMap(),
	}
}

func (this *PeersList) Add(address string) bool {
	this.Lock.Lock()
	defer this.Lock.Unlock()

	if _, exists := this.Queued.Get(address); exists {
		return false
	}
	if _, exists := this.Connected[address]; exists {
		return false
	}

	this.Queued.Set(address, &Peer{
		Address:              address,
		LastConnectTimestamp: 0,
		Attempts:             0,
		Errors:               0,
	})

	return true
}

func (this *PeersList) ScheduleReconnect(maxActive int) []*Peer {
	result := make([]*Peer, 0)

	this.Lock.Lock()
	defer this.Lock.Unlock()

	active := len(this.Connected)
	if active >= maxActive {
		return result
	}

	toAdd := maxActive - active
	current := uint32(time.Now().Unix())
	iter := this.Queued.IterFunc()
	for kv, ok := iter(); ok && toAdd > 0; kv, ok = iter() {
		address := kv.Key.(string)
		peer := kv.Value.(*Peer)
		if current < peer.LastConnectTimestamp+peer.ReconnectPenalty {
			continue
		}
		peer.LastConnectTimestamp = uint32(time.Now().Unix())
		peer.ReconnectPenalty = utils.MinUint32(defaults.ReconnectionDelayMax, peer.ReconnectPenalty+1)

		this.Queued.Delete(address)
		this.Connected[address] = peer

		result = append(result, peer)

		toAdd--
	}

	return result
}

func (this *PeersList) SetDisconnected(peer *Peer) {
	this.Lock.Lock()
	defer this.Lock.Unlock()
	delete(this.Connected, peer.Address)
	this.Queued.Set(peer.Address, peer)
}
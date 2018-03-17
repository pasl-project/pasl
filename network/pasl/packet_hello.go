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
	"strconv"
	"strings"
	"time"

	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/network"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/utils"
)

type helloHandler struct {
	Nonce              []byte
	GetTopBlock        func() safebox.BlockBase
	GetPeersByType     func(addressType network.AddressType) map[network.Address]*network.Peer
	GetUserAgentString func() string
	BlockUpdates       chan<- safebox.SerializedBlockHeader
	PeerUpdates        chan<- PeerInfo
}

type PeerInfo struct {
	Host        string
	Port        uint16
	LastConnect uint32
}

type packetHello struct {
	NodePort  uint16
	Nonce     []byte
	Time      uint32
	Block     safebox.SerializedBlockHeader
	Peers     []PeerInfo
	UserAgent string
}

func (this *helloHandler) getTcpPeersList() []PeerInfo {
	tcpPeers := this.GetPeersByType(network.TCP)
	peers := make([]PeerInfo, len(tcpPeers))
	i := 0
	for address, peer := range tcpPeers {
		hostPort := strings.Split(address.String()[len("tcp://"):], ":")
		port_, err := strconv.Atoi(hostPort[1])
		port := uint16(port_)
		if err != nil {
			port = defaults.P2PPort
		}
		peers[i] = PeerInfo{
			Host:        hostPort[0],
			Port:        port,
			LastConnect: peer.LastConnectTimestamp,
		}
		i++
	}
	return peers
}

func generateHello(nodePort uint16, nonce []byte, pendingBlock safebox.SerializedBlockHeader, peers []PeerInfo, userAgent string) []byte {
	return utils.Serialize(packetHello{
		NodePort:  nodePort,
		Nonce:     nonce,
		Time:      uint32(time.Now().Unix()),
		Block:     pendingBlock,
		Peers:     peers,
		UserAgent: userAgent,
	})
}

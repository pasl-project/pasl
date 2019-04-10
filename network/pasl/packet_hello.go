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
	"net/url"
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
	GetPeersByNetwork  func(network string) map[string]*network.Peer
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
	tcpPeers := this.GetPeersByNetwork("tcp")
	peers := make([]PeerInfo, len(tcpPeers))
	i := 0
	for address, peer := range tcpPeers {
		hostPort := strings.Split(address, ":")
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

func generateHello(nodePort uint16, nonce []byte, pendingBlock safebox.SerializedBlockHeader, peers map[string]network.Peer, userAgent string) []byte {
	whitePeers := make([]PeerInfo, 0, len(peers))
	for address := range peers {
		parsed, err := url.Parse(address)
		if err != nil {
			continue
		}
		port, err := strconv.ParseUint(parsed.Port(), 10, 16)
		if err != nil {
			continue
		}
		whitePeers = append(whitePeers, PeerInfo{
			Host:        parsed.Hostname(),
			Port:        uint16(port),
			LastConnect: peers[address].LastConnectTimestamp,
		})
	}

	return utils.Serialize(packetHello{
		NodePort:  nodePort,
		Nonce:     nonce,
		Time:      uint32(time.Now().Unix()),
		Block:     pendingBlock,
		Peers:     whitePeers,
		UserAgent: userAgent,
	})
}

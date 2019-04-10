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
	"time"

	"github.com/pasl-project/pasl/network"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/utils"
)

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

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

package defaults

import (
	"crypto/sha256"
	"fmt"
	"time"
)

const (
	VersionMajor uint16 = 0
	VersionMinor uint16 = 1
)

const (
	NetId                   uint32        = 0x5891E4FF
	BootstrapNodes          string        = "pascallite.ddns.net:4004,pascallite2.ddns.net:4004,pascallite3.ddns.net:4004,pascallite4.dynamic-dns.net:4004,pascallite5.dynamic-dns.net:4004,pascallite.dynamic-dns.net:4004,pascallite2.dynamic-dns.net:4004,pascallite3.dynamic-dns.net:4004"
	P2PBindAddress          string        = "0.0.0.0"
	P2PPort                 uint16        = 4004
	RPCBindAddress          string        = "127.0.0.1"
	RPCPort                 uint16        = 4003
	TimeoutConnect          time.Duration = time.Duration(10) * time.Second
	TimeoutRequest          time.Duration = time.Duration(60) * time.Second
	MaxIncoming             uint32        = 100
	MaxOutgoing             uint32        = 10
	NetworkBlocksPerRequest uint32        = 100
	ReconnectionDelayMax    uint32        = 30
)

const (
	GenesisReward        uint64 = 500000
	MinReward            uint64 = 10000
	RewardDecreaseBlocks uint32 = 420480
)

const (
	MinTarget        uint32 = 0x24000000
	MinTargetBits    uint   = uint(MinTarget >> 24)
	DifficultyBlocks uint32 = 10
)

const (
	AccountsPerBlock uint32 = 5
	MaturationHeight uint32 = 100
)

var UserAgent = fmt.Sprintf("PASL v%d.%d", VersionMajor, VersionMinor)
var GenesisSafeBox = sha256.Sum256([]byte("February 1 2017 - CNN - Trump puts on a flawless show in picking Gorsuch for Supreme Court "))
var GenesisPow = []byte{0x00, 0x00, 0x00, 0x00, 0x0E, 0xAE, 0x7A, 0x91, 0xB7, 0x48, 0xC7, 0x35, 0xA5, 0x33, 0x8A, 0x11, 0x71, 0x5D, 0x81, 0x51, 0x01, 0xE0, 0xC0, 0x75, 0xF7, 0xC6, 0x0F, 0xA5, 0x2B, 0x76, 0x9E, 0xC7}

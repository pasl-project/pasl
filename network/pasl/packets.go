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
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/safebox/tx"
)

type packetGetBlocksRequest struct {
	FromIndex uint32
	ToIndex   uint32
}

type packetGetBlocksResponse struct {
	Blocks []safebox.SerializedBlock
}

type packetGetHeadersResponse struct {
	BlockHeaders []safebox.SerializedBlockHeader
}

type packetError struct {
	Message string
}

type packetNewBlock struct {
	safebox.SerializedBlock
}

type packetNewOperations struct {
	tx.OperationsNetwork
}

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

package safebox

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

type antiHopDiff struct{}

func (this *antiHopDiff) CheckBlock(currentTarget common.TargetBase, block BlockBase) error {
	if !currentTarget.Equal(block.GetTarget()) {
		return fmt.Errorf("Invalid block #%d target 0x%08x != 0x%08x expected", block.GetIndex(), block.GetTarget().GetCompact(), currentTarget.GetCompact())
	}

	pow := this.GetBlockPow(block)
	if !currentTarget.Check(pow[:]) {
		return fmt.Errorf("POW check failed %s > %064s", hex.EncodeToString(pow[:]), currentTarget.Get().Text(16))
	}

	return nil
}

func (this *antiHopDiff) GetNextTarget(currentTarget common.TargetBase, getLastTimestamps GetLastTimestamps) uint32 {
	timestamps := getLastTimestamps(defaults.DifficultyBlocks + 1)
	if len(timestamps) < 2 {
		return currentTarget.GetCompact()
	}

	median := int64((timestamps[0] - timestamps[len(timestamps)-1]) / utils.MinUint32(defaults.DifficultyBlocks, uint32(len(timestamps)-1)))

	multiplier1 := utils.MaxInt64(0, 4-(median/50))
	multiplier1 = multiplier1 * multiplier1 * multiplier1

	multiplier2 := utils.MaxInt64(-86400, utils.MinInt64(0, 300-int64(median)))

	previous := big.NewInt(0)
	targetHash := big.NewInt(0).Set(currentTarget.Get())

	previous.Set(currentTarget.Get()).Div(previous, big.NewInt(512)).Mul(previous, big.NewInt(multiplier1))
	targetHash.Sub(targetHash, previous)

	previous.Set(currentTarget.Get()).Div(previous, big.NewInt(86400)).Mul(previous, big.NewInt(multiplier2))
	targetHash.Sub(targetHash, previous)

	return common.ToCompact(targetHash)
}

func (*antiHopDiff) GetBlockHashingBlob(block BlockBase) (template []byte, reservedOffset int) {
	return GetBlockHashingBlob(block)
}

func (a *antiHopDiff) GetBlockPow(block BlockBase) []byte {
	hashingBlob, _ := a.GetBlockHashingBlob(block)
	return GetBlockPow(hashingBlob)
}

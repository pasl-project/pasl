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

package safebox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

type antiHopDiff struct {
	Fork
}

func (this *antiHopDiff) CheckBlock(currentTarget common.TargetBase, block BlockBase) error {
	if !currentTarget.Equal(block.GetTarget()) {
		return fmt.Errorf("Invalid block #%d target 0x%08x != 0x%08x expected", block.GetIndex(), block.GetTarget().GetCompact(), currentTarget.GetCompact())
	}

	hashingBlob, _, _ := this.GetBlockHashingBlob(block)
	hash := sha256.Sum256(hashingBlob)
	pow := sha256.Sum256(hash[:])
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

func (this *antiHopDiff) GetBlockHashingBlob(block BlockBase) (template []byte, reservedOffset int, reservedSize int) {
	type part1 struct {
		Index   uint32
		Miner   crypto.PublicSerialized
		Reward  uint64
		Version common.Version
		Target  uint32
	}
	type part2 struct {
		PrevSafeboxHash utils.Serializable
		OperationsHash  utils.Serializable
		Fee             uint32
		Timestamp       uint32
		Nonce           uint32
	}
	toHash := utils.Serialize(part1{
		Index:   block.GetIndex(),
		Miner:   block.GetMiner().Serialized(),
		Reward:  block.GetReward(),
		Version: block.GetVersion(),
		Target:  block.GetTarget().GetCompact(),
	})

	payload := block.GetPayload()
	toHash = append(toHash, payload...)
	reservedOffset = len(toHash)
	reservedSize = len(payload)

	toHash = append(toHash, utils.Serialize(part2{
		PrevSafeboxHash: &utils.BytesWithoutLengthPrefix{
			Bytes: block.GetPrevSafeBoxHash(),
		},
		OperationsHash: &utils.BytesWithoutLengthPrefix{
			Bytes: block.GetOperationsHash(),
		},
		Fee:       uint32(block.GetFee()),
		Timestamp: block.GetTimestamp(),
		Nonce:     block.GetNonce(),
	})...)

	return toHash, reservedOffset, reservedSize
}

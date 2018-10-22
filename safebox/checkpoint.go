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
	"crypto/sha256"

	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/utils"
)

type checkpoint struct {
	Fork
}

func (this *checkpoint) CheckBlock(currentTarget common.TargetBase, block BlockBase) error {
	return nil
}

func (this *checkpoint) GetNextTarget(currentTarget common.TargetBase, getLastTimestamps GetLastTimestamps) uint32 {
	return currentTarget.GetCompact()
}

// TODO: remove code duplication
func (this *checkpoint) GetBlockHashingBlob(block BlockBase) (template []byte, reservedOffset int, reservedSize int) {
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

func (this *checkpoint) GetBlockPow(block BlockBase) []byte {
	hashingBlob, _, _ := this.GetBlockHashingBlob(block)
	hash := sha256.Sum256(hashingBlob)
	pow := sha256.Sum256(hash[:])
	return pow[:]
}

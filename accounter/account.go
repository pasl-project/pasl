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

package accounter

import (
	"encoding/hex"
	"strconv"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/utils"
)

type Account struct {
	Number       uint32
	PublicKey    crypto.Public
	Balance      uint64
	UpdatedIndex uint32
	Operations   uint32
	Timestamp    uint32
}

type AccountHashBuffer struct {
	Number       uint32
	PublicKey    crypto.Public
	Balance      uint64
	UpdatedIndex uint32
	Operations   uint32
}

func (this *Account) GetHashBuffer() AccountHashBuffer {
	return AccountHashBuffer{
		Number:       this.Number,
		PublicKey:    this.PublicKey,
		Balance:      this.Balance,
		UpdatedIndex: this.UpdatedIndex,
		Operations:   this.Operations,
	}
}

func (this *Account) GetTimestamp() uint32 {
	return this.Timestamp
}

func (this *Account) BalanceSub(amount uint64, index uint32) []Micro {
	newBalance := this.Balance - amount
	newOperations := this.Operations + 1

	result := []Micro{
		Micro{
			Opcode:   CompareSwapBalance,
			ValueOld: strconv.FormatUint(this.Balance, 10),
			ValueNew: strconv.FormatUint(newBalance, 10),
		},
		Micro{
			Opcode:   CompareSwapUpdatedIndex,
			ValueOld: strconv.FormatUint(uint64(this.UpdatedIndex), 10),
			ValueNew: strconv.FormatUint(uint64(index), 10),
		},
		Micro{
			Opcode:   CompareSwapOperations,
			ValueOld: strconv.FormatUint(uint64(this.Operations), 10),
			ValueNew: strconv.FormatUint(uint64(newOperations), 10),
		},
	}

	this.Balance = newBalance
	this.UpdatedIndex = index
	this.Operations = newOperations

	return result
}

func (this *Account) BalanceAdd(amount uint64, index uint32) []Micro {
	newBalance := this.Balance + amount

	result := []Micro{
		Micro{
			Opcode:   CompareSwapBalance,
			ValueOld: strconv.FormatUint(this.Balance, 10),
			ValueNew: strconv.FormatUint(newBalance, 10),
		},
		Micro{
			Opcode:   CompareSwapUpdatedIndex,
			ValueOld: strconv.FormatUint(uint64(this.UpdatedIndex), 10),
			ValueNew: strconv.FormatUint(uint64(index), 10),
		},
	}

	this.Balance = newBalance
	this.UpdatedIndex = index

	return result
}

func (this *Account) KeyChange(key *crypto.Public, index uint32) []Micro {
	result := []Micro{
		Micro{
			Opcode:   CompareSwapKey,
			ValueOld: hex.EncodeToString(utils.Serialize(&this.PublicKey)),
			ValueNew: hex.EncodeToString(utils.Serialize(key)),
		},
		Micro{
			Opcode:   CompareSwapUpdatedIndex,
			ValueOld: strconv.FormatUint(uint64(this.UpdatedIndex), 10),
			ValueNew: strconv.FormatUint(uint64(index), 10),
		},
	}

	this.PublicKey = *key
	this.UpdatedIndex = index

	return result
}

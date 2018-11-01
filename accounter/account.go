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

package accounter

import (
	"github.com/pasl-project/pasl/crypto"
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

func (this *Account) BalanceSub(amount uint64, index uint32) {
	this.Balance = this.Balance - amount
	this.UpdatedIndex = index
	this.Operations = this.Operations + 1
}

func (this *Account) BalanceAdd(amount uint64, index uint32) {
	this.Balance = this.Balance + amount
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

func (this *Account) KeyChange(key *crypto.Public, index uint32) {
	this.PublicKey = *key
	this.UpdatedIndex = index

	return result
}

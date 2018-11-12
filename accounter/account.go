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
	"io"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/utils"
)

type Account struct {
	number          uint32
	publicKey       crypto.Public
	balance         uint64
	updatedIndex    uint32
	operations      uint32
	operationsTotal uint32
	timestamp       uint32
}

type accountSerialized struct {
	Number          uint32
	PublicKey       crypto.PublicSerialized
	Balance         uint64
	UpdatedIndex    uint32
	Operations      uint32
	OperationsTotal uint32
	Timestamp       uint32
}

type AccountHashBuffer struct {
	Number       uint32
	PublicKey    crypto.Public
	Balance      uint64
	UpdatedIndex uint32
	Operations   uint32
}

func NewAccount(number uint32, publicKey *crypto.Public, balance uint64, updatedIndex uint32, operationsCount uint32, operationsTotal uint32) Account {
	return Account{
		number:          number,
		publicKey:       *publicKey,
		balance:         balance,
		updatedIndex:    updatedIndex,
		operations:      operationsCount,
		operationsTotal: operationsTotal,
	}
}

func (this *Account) GetHashBuffer() AccountHashBuffer {
	return AccountHashBuffer{
		Number:       this.number,
		PublicKey:    this.publicKey,
		Balance:      this.balance,
		UpdatedIndex: this.updatedIndex,
		Operations:   this.operations,
	}
}

func (this *Account) GetBalance() uint64 {
	return this.balance
}

func (this *Account) GetNumber() uint32 {
	return this.number
}

func (this *Account) GetOperationsCount() uint32 {
	return this.operations
}

func (this *Account) GetOperationsTotal() uint32 {
	return this.operationsTotal
}

func (this *Account) GetPublicKeySerialized() crypto.PublicSerialized {
	return this.publicKey.Serialized()
}

func (this *Account) GetTimestamp() uint32 {
	return this.timestamp
}

func (this *Account) GetUpdatedIndex() uint32 {
	return this.updatedIndex
}

func (this *Account) IsPublicKeyEqual(other *crypto.Public) bool {
	return this.publicKey.Equal(other)
}

func (this *Account) Serialize(w io.Writer) error {
	_, err := w.Write(utils.Serialize(utils.Serialize(accountSerialized{
		Number:          this.number,
		PublicKey:       this.publicKey.Serialized(),
		Balance:         this.balance,
		UpdatedIndex:    this.updatedIndex,
		Operations:      this.operations,
		OperationsTotal: this.operationsTotal,
		Timestamp:       this.timestamp,
	})))

	return err
}

func (this *Account) Deserialize(r io.Reader) error {
	var unpacked accountSerialized
	if err := utils.Deserialize(&unpacked, r); err != nil {
		return err
	}

	this.number = unpacked.Number
	crypto.PublicFromSerialized(&this.publicKey, &unpacked.PublicKey)
	this.balance = unpacked.Balance
	this.updatedIndex = unpacked.UpdatedIndex
	this.operations = unpacked.Operations
	this.operationsTotal = unpacked.OperationsTotal
	this.timestamp = unpacked.Timestamp

	return nil
}

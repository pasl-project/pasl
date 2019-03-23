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
	number          uint32
	publicKey       crypto.Public
	balance         uint64
	updatedIndex    uint32
	operations      uint32
	operationsTotal uint32
	timestamp       uint32
}

type AccountHashBuffer struct {
	Number       uint32
	PublicKey    crypto.Public
	Balance      uint64
	UpdatedIndex uint32
	Operations   uint32
}

func NewAccount(number uint32, publicKey *crypto.Public, balance uint64, updatedIndex uint32, operationsCount uint32, operationsTotal uint32, timestamp uint32) Account {
	return Account{
		number:          number,
		publicKey:       *publicKey,
		balance:         balance,
		updatedIndex:    updatedIndex,
		operations:      operationsCount,
		operationsTotal: operationsTotal,
		timestamp:       timestamp,
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

func (a *Account) FromPod(pod AccountPod) {
	a.number = pod.Number
	crypto.PublicFromSerialized(&a.publicKey, pod.PublicKey.TypeId, pod.PublicKey.X, pod.PublicKey.Y)
	a.balance = pod.Balance
	a.updatedIndex = pod.UpdatedIndex
	a.operations = pod.Operations
	a.operationsTotal = pod.OperationsTotal
	a.timestamp = pod.Timestamp
}

func (this Account) Pod() *AccountPod {
	return &AccountPod{
		Number: this.number,
		PublicKey: &PublicPod{
			TypeId: this.publicKey.TypeId,
			X:      this.publicKey.X.Bytes(),
			Y:      this.publicKey.Y.Bytes(),
		},
		Balance:         this.balance,
		UpdatedIndex:    this.updatedIndex,
		Operations:      this.operations,
		OperationsTotal: this.operationsTotal,
		Timestamp:       this.timestamp,
	}
}

func (this Account) Marshal() ([]byte, error) {
	return this.Pod().MarshalBinary()
}

func (this *Account) Unmarshal(data []byte) (int, error) {
	pod := AccountPod{}
	size, err := pod.Unmarshal(data)
	if err != nil {
		return 0, err
	}

	this.FromPod(pod)
	return size, nil
}

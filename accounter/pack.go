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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math/big"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

type PackBase struct {
	accounts             []Account
	cumulativeDifficulty *big.Int
	dirty                bool
	hash                 [sha256.Size]byte
	index                uint32
}

type packSerialized struct {
	Accounts             []Account
	CumulativeDifficulty []byte
	Dirty                bool
	Hash                 [sha256.Size]byte
	Index                uint32
}

func NewPackWithAccounts(index uint32, accounts []Account, cumulativeDifficulty *big.Int) PackBase {
	accountsCopy := make([]Account, len(accounts))
	copy(accountsCopy, accounts)
	return PackBase{
		accounts:             accountsCopy,
		cumulativeDifficulty: big.NewInt(0).Set(cumulativeDifficulty),
		dirty:                true,
		index:                index,
	}
}

func (p *PackBase) Copy() PackBase {
	return NewPackWithAccounts(p.index, p.accounts, p.cumulativeDifficulty)
}

func NewPack(index uint32, miner *crypto.Public, reward uint64, timestamp uint32, cumulativeDifficulty *big.Int) PackBase {
	accounts := make([]Account, defaults.AccountsPerBlock)
	number := index * uint32(defaults.AccountsPerBlock)
	for i := range accounts {
		accounts[i] = NewAccount(number, miner, 0, index, 0, 0, timestamp)
		number++
	}
	accounts[0].balance = reward

	return NewPackWithAccounts(index, accounts, cumulativeDifficulty)
}

func (this *PackBase) ToBlob() []byte {
	buf := bytes.NewBuffer([]byte(""))
	binary.Write(buf, binary.LittleEndian, this.index)
	for _, it := range this.accounts {
		binary.Write(buf, binary.LittleEndian, it.number)
		binary.Write(buf, binary.LittleEndian, utils.Serialize(&it.publicKey))
		binary.Write(buf, binary.LittleEndian, it.balance)
		binary.Write(buf, binary.LittleEndian, it.updatedIndex)
		binary.Write(buf, binary.LittleEndian, it.operations)
	}
	binary.Write(buf, binary.LittleEndian, this.accounts[0].GetTimestamp())
	return buf.Bytes()
}

func (this *PackBase) GetHash() []byte {
	if !this.dirty {
		return this.hash[:]
	}

	// TODO: generalized serialization
	this.hash = sha256.Sum256(this.ToBlob())
	this.dirty = false
	return this.hash[:]
}

func (this *PackBase) GetIndex() uint32 {
	return this.index
}

func (this *PackBase) GetAccount(offset int) *Account {
	return &this.accounts[offset]
}

func (this *PackBase) GetCumulativeDifficulty() *big.Int {
	return big.NewInt(0).Set(this.cumulativeDifficulty)
}

func (this *PackBase) BalanceSub(offset uint32, amount uint64, index uint32) {
	this.dirty = true

	account := &this.accounts[offset]
	account.balance = account.balance - amount
	account.operations = account.operations + 1
	account.operationsTotal = account.operationsTotal + 1
	account.updatedIndex = index
}

func (this *PackBase) BalanceAdd(offset uint32, amount uint64, index uint32) {
	this.dirty = true

	account := &this.accounts[offset]
	account.balance = account.balance + amount
	account.updatedIndex = index
	account.operationsTotal = account.operationsTotal + 1
}

func (this *PackBase) KeyChange(offset uint32, key *crypto.Public, index uint32, fee uint64) {
	this.dirty = true

	account := &this.accounts[offset]
	account.balance = account.balance - fee
	account.operations = account.operations + 1
	account.operationsTotal = account.operationsTotal + 1
	account.publicKey = *key
	account.updatedIndex = index
}

func (p *PackBase) FromPod(pod PackPod) {
	accounts := make([]Account, len(pod.Accounts))
	for each := range accounts {
		a := Account{}
		a.FromPod(*pod.Accounts[each])
		accounts[each] = a
	}

	p.accounts = accounts
	p.cumulativeDifficulty = big.NewInt(0).SetBytes(pod.CumulativeDifficulty)
	p.dirty = pod.Dirty
	p.index = pod.Index
	copy(p.hash[:sha256.Size], pod.Hash[:sha256.Size])
}

func (this *PackBase) Pod() *PackPod {
	accounts := make([]*AccountPod, len(this.accounts))
	for each := range accounts {
		accounts[each] = this.accounts[each].Pod()
	}

	return &PackPod{
		Accounts:             accounts,
		CumulativeDifficulty: this.cumulativeDifficulty.Bytes(),
		Dirty:                this.dirty,
		Hash:                 this.hash[:],
		Index:                this.index,
	}
}

func (p *PackBase) Marshal() ([]byte, error) {
	return p.Pod().MarshalBinary()
}

func (p *PackBase) Unmarshal(data []byte) (int, error) {
	pod := PackPod{}
	size, err := pod.Unmarshal(data)
	if err != nil {
		return 0, err
	}

	p.FromPod(pod)
	return size, nil
}

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

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

type pack struct {
	index    uint32
	accounts []*Account
	hash     [32]byte
	dirty    bool
}

type packBase interface {
	GetHash() []byte
	GetAccounts() []*Account
	MarkDirty()
}

func NewPackWithAccounts(index uint32, accounts []*Account) packBase {
	accountsCopy := make([]*Account, len(accounts))
	copy(accountsCopy, accounts)
	return &pack{
		index:    index,
		accounts: accountsCopy,
		dirty:    true,
	}
}

func NewPack(index uint32, miner *crypto.Public, timestamp uint32) packBase {
	accounts := make([]*Account, defaults.AccountsPerBlock)
	number := index * uint32(defaults.AccountsPerBlock)
	for i, _ := range accounts {
		accounts[i] = &Account{
			Number:          number,
			PublicKey:       *miner,
			Balance:         0,
			UpdatedIndex:    index,
			Operations:      0,
			OperationsTotal: 0,
			Timestamp:       timestamp,
		}
		number++
	}

	return NewPackWithAccounts(index, accounts)
}

func (this *pack) GetHash() []byte {
	if !this.dirty {
		return this.hash[:]
	}

	// TODO: generalized serialization
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, this.index)
	for _, it := range this.accounts {
		binary.Write(buf, binary.LittleEndian, it.Number)
		binary.Write(buf, binary.LittleEndian, utils.Serialize(&it.PublicKey))
		binary.Write(buf, binary.LittleEndian, it.Balance)
		binary.Write(buf, binary.LittleEndian, it.UpdatedIndex)
		binary.Write(buf, binary.LittleEndian, it.Operations)
	}
	binary.Write(buf, binary.LittleEndian, this.accounts[0].GetTimestamp())
	this.hash = sha256.Sum256(buf.Bytes())
	this.dirty = false
	return this.hash[:]
}

func (this *pack) GetAccounts() []*Account {
	return this.accounts
}

func (this *pack) MarkDirty() {
	this.dirty = true
}

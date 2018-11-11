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
	"io"
	"math/big"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

type PackBase struct {
	accounts             []Account
	cumulativeDifficulty *big.Int
	dirty                bool
	hash                 [32]byte
	index                uint32
}

type packSerialized struct {
	Accounts             []Account
	CumulativeDifficulty []byte
	Dirty                bool
	Hash                 [32]byte
	Index                uint32
}

func NewPackWithAccounts(index uint32, accounts []Account, cumulativeDifficulty *big.Int) *PackBase {
	accountsCopy := make([]Account, len(accounts))
	copy(accountsCopy, accounts)
	return &PackBase{
		accounts:             accountsCopy,
		cumulativeDifficulty: big.NewInt(0).Set(cumulativeDifficulty),
		dirty:                true,
		index:                index,
	}
}

func NewPack(index uint32, miner *crypto.Public, timestamp uint32, cumulativeDifficulty *big.Int) *PackBase {
	accounts := make([]Account, defaults.AccountsPerBlock)
	number := index * uint32(defaults.AccountsPerBlock)
	for i, _ := range accounts {
		accounts[i] = Account{
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

	return NewPackWithAccounts(index, accounts, cumulativeDifficulty)
}

func (this *PackBase) ToBlob() []byte {
	buf := bytes.NewBuffer([]byte(""))
	binary.Write(buf, binary.LittleEndian, this.index)
	for _, it := range this.accounts {
		binary.Write(buf, binary.LittleEndian, it.Number)
		binary.Write(buf, binary.LittleEndian, utils.Serialize(&it.PublicKey))
		binary.Write(buf, binary.LittleEndian, it.Balance)
		binary.Write(buf, binary.LittleEndian, it.UpdatedIndex)
		binary.Write(buf, binary.LittleEndian, it.Operations)
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
	return this.cumulativeDifficulty
}

func (this *PackBase) MarkDirty() {
	this.dirty = true
}

func (this *PackBase) Serialize(w io.Writer) error {
	_, err := w.Write(utils.Serialize(utils.Serialize(packSerialized{
		Accounts:             this.accounts,
		CumulativeDifficulty: this.cumulativeDifficulty.Bytes(),
		Dirty:                this.dirty,
		Hash:                 this.hash,
		Index:                this.index,
	})))
	return err
}

func (this *PackBase) Deserialize(r io.Reader) error {
	var unpacked packSerialized
	if err := utils.Deserialize(&unpacked, r); err != nil {
		return err
	}
	this.accounts = unpacked.Accounts
	this.cumulativeDifficulty = big.NewInt(0).SetBytes(unpacked.CumulativeDifficulty)
	this.dirty = unpacked.Dirty
	this.hash = unpacked.Hash
	this.index = unpacked.Index
	return nil
}

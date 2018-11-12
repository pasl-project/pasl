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
	"math/big"
	"sync"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
)

type Accounter struct {
	dirty bool
	hash  []byte
	lock  sync.RWMutex
	packs []*PackBase
}

func NewAccounter() *Accounter {
	hash := make([]byte, 32)
	copy(hash[:], defaults.GenesisSafeBox[:])

	return &Accounter{
		dirty: false,
		hash:  hash,
		packs: make([]*PackBase, 0),
	}
}

func (this *Accounter) Copy() *Accounter {
	this.lock.RLock()
	defer this.lock.RUnlock()

	hash := make([]byte, 32)
	copy(hash[:], this.hash)

	packs := make([]*PackBase, len(this.packs))
	copy(packs[:], this.packs)

	return &Accounter{
		dirty: this.dirty,
		hash:  hash,
		packs: packs,
	}
}

func (this *Accounter) ToBlob() []byte {
	this.lock.RLock()
	defer this.lock.RUnlock()

	result := bytes.NewBuffer([]byte(""))
	for _, pack := range this.packs {
		result.Write(pack.ToBlob())
		result.Write(pack.GetHash())
	}
	return result.Bytes()
}

func (this *Accounter) getHashUnsafe() []byte {
	if !this.dirty {
		return this.hash[:]
	}
	this.dirty = false

	hash := sha256.New()
	for _, it := range this.packs {
		hash.Write(it.GetHash())
	}
	copy(this.hash[:32], hash.Sum(nil)[:32])
	return this.hash[:]
}

func (this *Accounter) getHeightUnsafe() uint32 {
	return uint32(len(this.packs))
}

func (this *Accounter) GetState() (uint32, []byte, *big.Int) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getHeightUnsafe(), this.getHashUnsafe(), this.getCumulativeDifficultyUnsafe()
}

func (this *Accounter) getCumulativeDifficultyUnsafe() *big.Int {
	if len(this.packs) > 0 {
		return this.packs[len(this.packs)-1].GetCumulativeDifficulty()
	}
	return big.NewInt(0)
}

func (this *Accounter) getPackContainingAccountUnsafe(number uint32) *PackBase {
	pack := number / uint32(defaults.AccountsPerBlock)
	return this.packs[pack]
}

func (this *Accounter) GetCumulativeDifficultyAndTimestamp(index uint32) (*big.Int, uint32) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	pack := this.getPackContainingAccountUnsafe(index)
	return pack.GetCumulativeDifficulty(), pack.GetAccount(0).GetTimestamp()
}

func (this *Accounter) GetAccount(number uint32) *Account {
	this.lock.RLock()
	defer this.lock.RUnlock()

	offset := number % defaults.AccountsPerBlock
	return this.getPackContainingAccountUnsafe(number).GetAccount(int(offset))
}

func (this *Accounter) MarkAccountDirty(number uint32) *PackBase {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.dirty = true
	pack := this.getPackContainingAccountUnsafe(number)
	pack.MarkDirty()

	return pack
}

func (this *Accounter) appendPackUnsafe(pack *PackBase) {
	this.packs = append(this.packs, pack)
	this.dirty = true
}

func (this *Accounter) AppendPack(pack *PackBase) {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.appendPackUnsafe(pack)
}

func (this *Accounter) NewPack(miner *crypto.Public, reward uint64, timestamp uint32, difficulty *big.Int) *PackBase {
	this.lock.Lock()
	defer this.lock.Unlock()

	cumulativeDifficulty := big.NewInt(0).Add(this.getCumulativeDifficultyUnsafe(), difficulty)
	pack := NewPack(this.getHeightUnsafe(), miner, reward, timestamp, cumulativeDifficulty)
	this.appendPackUnsafe(pack)
	return pack
}

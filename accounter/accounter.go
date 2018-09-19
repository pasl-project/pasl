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
	"crypto/sha256"
	"sync"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
)

type Accounter struct {
	hash  []byte
	packs []packBase
	dirty bool
	lock  sync.RWMutex
}

func NewAccounter() *Accounter {
	hash := make([]byte, 32)
	copy(hash[:], defaults.GenesisSafeBox[:])

	return &Accounter{
		hash:  hash,
		packs: make([]packBase, 0),
		dirty: false,
	}
}

func (this *Accounter) Copy() *Accounter {
	this.lock.RLock()
	defer this.lock.RUnlock()

	hash := make([]byte, 32)
	copy(hash[:], this.hash)

	packs := make([]packBase, len(this.packs))
	copy(packs[:], this.packs)

	return &Accounter{
		hash:  hash,
		packs: packs,
		dirty: this.dirty,
	}
}

func (this *Accounter) getHashUnsafe() []byte {
	if !this.dirty {
		return this.hash[:]
	}

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

func (this *Accounter) GetState() (uint32, []byte) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getHeightUnsafe(), this.getHashUnsafe()
}

func (this *Accounter) getPackContainingAccountUnsafe(number uint32) packBase {
	pack := number / uint32(defaults.AccountsPerBlock)
	return this.packs[pack]
}

func (this *Accounter) GetAccount(number uint32) *Account {
	this.lock.RLock()
	defer this.lock.RUnlock()

	offset := number % uint32(defaults.AccountsPerBlock)
	return this.getPackContainingAccountUnsafe(number).GetAccounts()[offset]
}

func (this *Accounter) MarkAccountDirty(number uint32) {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.dirty = true
	this.getPackContainingAccountUnsafe(number).MarkDirty()
}

func (this *Accounter) appendPackUnsafe(pack packBase) []*Account {
	this.packs = append(this.packs, pack)
	this.dirty = true
	return pack.GetAccounts()
}

func (this *Accounter) AppendPack(pack packBase) []*Account {
	this.lock.Lock()
	defer this.lock.Unlock()

	return this.appendPackUnsafe(pack)
}

func (this *Accounter) NewPack(miner *crypto.Public, timestamp uint32) (newAccounts []*Account, newIndex uint32) {
	this.lock.Lock()
	defer this.lock.Unlock()

	newIndex = this.getHeightUnsafe()
	pack := NewPack(this.getHeightUnsafe(), miner, timestamp)
	return this.appendPackUnsafe(pack), newIndex
}

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
	"io"
	"math/big"
	"sync"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

type Accounter struct {
	dirty      map[int]struct{}
	hash       []byte
	hashBuffer []byte
	lock       sync.RWMutex
	packs      []PackBase
}

type accounterSerialized struct {
	Packs []PackBase
}

func NewAccounter() *Accounter {
	hash := make([]byte, sha256.Size)
	copy(hash[:], defaults.GenesisSafeBox[:])

	return &Accounter{
		dirty:      make(map[int]struct{}),
		hash:       hash,
		hashBuffer: make([]byte, 0),
		packs:      make([]PackBase, 0),
	}
}

func (this *Accounter) Copy() *Accounter {
	this.lock.RLock()
	defer this.lock.RUnlock()

	hash := make([]byte, sha256.Size)
	copy(hash[:], this.hash)

	hashBuffer := make([]byte, len(this.hashBuffer))
	copy(hashBuffer[:], this.hashBuffer)

	packs := make([]PackBase, len(this.packs))
	copy(packs[:], this.packs)

	dirty := make(map[int]struct{})
	for each := range this.dirty {
		dirty[each] = struct{}{}
	}

	return &Accounter{
		dirty:      dirty,
		hash:       hash,
		hashBuffer: hashBuffer,
		packs:      packs,
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
	if len(this.dirty) == 0 {
		return this.hash[:]
	}

	for packIndex := range this.dirty {
		copy(this.hashBuffer[packIndex*sha256.Size:(packIndex+1)*sha256.Size], this.packs[packIndex].GetHash())
	}
	this.dirty = make(map[int]struct{})

	hash := sha256.Sum256(this.hashBuffer)
	copy(this.hash[:sha256.Size], hash[:])
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
	return &this.packs[pack]
}

func (this *Accounter) GetCumulativeDifficultyAndTimestamp(index uint32) (*big.Int, uint32) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	pack := this.packs[index]
	return pack.GetCumulativeDifficulty(), pack.GetAccount(0).GetTimestamp()
}

func (this *Accounter) getAccountUnsafe(number uint32) *Account {
	offset := number % defaults.AccountsPerBlock
	return this.getPackContainingAccountUnsafe(number).GetAccount(int(offset))
}

func (this *Accounter) GetAccount(number uint32) *Account {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getAccountUnsafe(number)
}

func (this *Accounter) GetAccountPack(number uint32) uint32 {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getPackContainingAccountUnsafe(number).GetIndex()
}

func (this *Accounter) GetAccountPackSerialized(index uint32) []byte {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return utils.Serialize(&this.packs[index])
}

func (this *Accounter) markAccountDirtyUnsafe(number uint32) {
	pack := this.getPackContainingAccountUnsafe(number)
	pack.MarkDirty()
	this.dirty[int(pack.GetIndex())] = struct{}{}
}

func (this *Accounter) appendPackUnsafe(pack PackBase) {
	this.packs = append(this.packs, pack)
	this.hashBuffer = append(this.hashBuffer, make([]byte, 32)...)
	this.dirty[len(this.packs)-1] = struct{}{}
}

func (this *Accounter) AppendPack(pack PackBase) {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.appendPackUnsafe(pack)
}

func (this *Accounter) NewPack(miner *crypto.Public, reward uint64, timestamp uint32, difficulty *big.Int) uint32 {
	this.lock.Lock()
	defer this.lock.Unlock()

	cumulativeDifficulty := this.getCumulativeDifficultyUnsafe()
	cumulativeDifficulty.Add(cumulativeDifficulty, difficulty)
	pack := NewPack(this.getHeightUnsafe(), miner, reward, timestamp, cumulativeDifficulty)
	this.appendPackUnsafe(pack)
	return pack.GetIndex()
}

func (this *Accounter) BalanceSub(number uint32, amount uint64, index uint32) {
	this.lock.Lock()
	defer this.lock.Unlock()
	defer this.markAccountDirtyUnsafe(number)

	account := this.getAccountUnsafe(number)
	account.balance = account.balance - amount
	account.operations = account.operations + 1
	account.operationsTotal = account.operationsTotal + 1
	account.updatedIndex = index
}

func (this *Accounter) BalanceAdd(number uint32, amount uint64, index uint32) {
	this.lock.Lock()
	defer this.lock.Unlock()
	defer this.markAccountDirtyUnsafe(number)

	account := this.getAccountUnsafe(number)
	account.balance = account.balance + amount
	account.updatedIndex = index
	account.operationsTotal = account.operationsTotal + 1
}

func (this *Accounter) KeyChange(number uint32, key *crypto.Public, index uint32, fee uint64) {
	this.lock.Lock()
	defer this.lock.Unlock()
	defer this.markAccountDirtyUnsafe(number)

	account := this.getAccountUnsafe(number)
	account.balance = account.balance - fee
	account.operations = account.operations + 1
	account.operationsTotal = account.operationsTotal + 1
	account.publicKey = *key
	account.updatedIndex = index
}

func (this *Accounter) Serialize(w io.Writer) error {
	this.lock.RLock()
	defer this.lock.RUnlock()

	_, err := w.Write(utils.Serialize(accounterSerialized{
		Packs: this.packs,
	}))
	return err
}

func (this *Accounter) Deserialize(r io.Reader) error {
	this.lock.RLock()
	defer this.lock.RUnlock()

	var unpacked accounterSerialized
	if err := utils.Deserialize(&unpacked, r); err != nil {
		return err
	}
	this.packs = unpacked.Packs
	this.hashBuffer = make([]byte, len(this.packs)*sha256.Size)
	for packIndex := range this.packs {
		copy(this.hashBuffer[packIndex*sha256.Size:(packIndex*+1)*sha256.Size], this.packs[packIndex].GetHash())
	}
	this.dirty = make(map[int]struct{})
	return nil
}

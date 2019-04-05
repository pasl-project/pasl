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
	"github.com/pasl-project/pasl/utils"
)

var emptyDigest = [sha256.Size]byte{}

type Accounter struct {
	hash       []byte
	hashBuffer []byte
	lock       sync.RWMutex
	packs      packsMap
	updated    packsMap
	dirty      map[uint32]struct{}
}

type accounterSerialized struct {
	Packs []PackBase
}

func NewAccounter() *Accounter {
	hash := make([]byte, sha256.Size)
	copy(hash[:], defaults.GenesisSafeBox[:])

	return &Accounter{
		hash:       hash,
		hashBuffer: make([]byte, 0),
		packs:      newPacksMap(),
		updated:    newPacksMap(),
		dirty:      make(map[uint32]struct{}),
	}
}

func (this *Accounter) getMaxPack() *uint32 {
	maxUpdated := this.updated.getMax()
	maxPacks := this.packs.getMax()
	if maxUpdated == nil {
		return maxPacks
	}
	if maxPacks == nil {
		return maxUpdated
	}
	max := utils.MaxUint32(*maxUpdated, *maxPacks)
	return &max
}

func (this *Accounter) ToBlob() []byte {
	this.lock.RLock()
	defer this.lock.RUnlock()

	result := bytes.NewBuffer([]byte(""))

	maxPack := this.getMaxPack()
	if maxPack != nil {
		for packIndex := uint32(0); packIndex <= *maxPack; packIndex++ {
			pack := this.getPackUnsafe(packIndex)
			result.Write(pack.ToBlob())
			result.Write(pack.GetHash())
		}
	}
	return result.Bytes()
}

func (a *Accounter) getHashUnsafe() []byte {
	if len(a.dirty) == 0 {
		hash := make([]byte, sha256.Size)
		copy(hash[:], a.hash[:sha256.Size])
		return hash[:]
	}

	totalPacks := a.getHeightUnsafe()

	bufferLength := int(totalPacks * sha256.Size)
	for len(a.hashBuffer) < bufferLength {
		a.hashBuffer = append(a.hashBuffer, emptyDigest[:]...)
	}

	for number := range a.dirty {
		if number >= totalPacks {
			continue
		}
		pack := a.getPackUnsafe(number)
		begin := number * sha256.Size
		end := begin + sha256.Size
		copy(a.hashBuffer[begin:end], pack.GetHash())
	}
	a.dirty = make(map[uint32]struct{})

	hash := sha256.Sum256(a.hashBuffer[:bufferLength])
	copy(a.hash[:sha256.Size], hash[:])
	return hash[:]
}

func (this *Accounter) getHeightUnsafe() uint32 {
	maxPack := this.getMaxPack()
	if maxPack == nil {
		return 0
	}
	return *maxPack + 1
}

func (a *Accounter) Merge() {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.updated.forEach(func(number uint32, pack *PackBase) {
		a.packs.set(number, pack)
	})
	a.updated = newPacksMap()
}

func (a *Accounter) Rollback() {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.updated.forEach(func(number uint32, pack *PackBase) {
		a.dirty[number] = struct{}{}
	})
	a.updated = newPacksMap()
}

func (this *Accounter) GetHeight() uint32 {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getHeightUnsafe()
}

func (this *Accounter) GetState() (uint32, []byte, *big.Int) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getHeightUnsafe(), this.getHashUnsafe(), this.getCumulativeDifficultyUnsafe()
}

func (this *Accounter) getCumulativeDifficultyUnsafe() *big.Int {
	max := this.getMaxPack()
	if max == nil {
		return big.NewInt(0)
	}
	pack := this.getPackUnsafe(*max)
	return pack.GetCumulativeDifficulty()
}

func (a *Accounter) getPackUnsafe(packNumber uint32) *PackBase {
	if pack := a.updated.get(packNumber); pack != nil {
		return pack
	}
	return a.packs.get(packNumber)
}

func (a *Accounter) getPackContainingAccountUnsafe(accountNumber uint32) *PackBase {
	packNumber := accountNumber / uint32(defaults.AccountsPerBlock)
	return a.getPackUnsafe(packNumber)
}

func (this *Accounter) GetCumulativeDifficultyAndTimestamp(index uint32) (*big.Int, uint32) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	pack := this.getPackUnsafe(index)
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

func (this *Accounter) GetAccountPackSerialized(index uint32) ([]byte, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	pack := this.getPackUnsafe(index)
	return pack.Marshal()
}

func (a *Accounter) GetUpdatedPacks() []uint32 {
	a.lock.RLock()
	defer a.lock.RUnlock()

	return a.updated.keys()
}

func (a *Accounter) getPackForUpdateUnsafe(accountNumber uint32) (pack *PackBase, offset uint32) {
	packNumber := accountNumber / uint32(defaults.AccountsPerBlock)
	pack = a.updated.get(packNumber)
	if pack == nil {
		pack = a.packs.get(packNumber).Copy()
		a.updated.set(packNumber, pack)
	}
	a.dirty[packNumber] = struct{}{}
	offset = accountNumber % defaults.AccountsPerBlock
	return pack, offset
}

func (a *Accounter) appendPackUnsafe(pack *PackBase) {
	packNumber := a.getHeightUnsafe()
	a.updated.set(packNumber, pack)
	a.dirty[packNumber] = struct{}{}
}

func (this *Accounter) AppendPack(pack *PackBase) {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.appendPackUnsafe(pack)
}

func (this *Accounter) NewPack(miner *crypto.Public, reward uint64, timestamp uint32, difficulty *big.Int) uint32 {
	this.lock.Lock()
	defer this.lock.Unlock()

	cumulativeDifficulty := this.getCumulativeDifficultyUnsafe()
	cumulativeDifficulty.Add(cumulativeDifficulty, difficulty)
	minerCopy := miner.Copy()
	pack := NewPack(this.getHeightUnsafe(), &minerCopy, reward, timestamp, cumulativeDifficulty)
	this.appendPackUnsafe(pack)
	return pack.GetIndex()
}

func (this *Accounter) BalanceSub(number uint32, amount uint64, index uint32) {
	this.lock.Lock()
	defer this.lock.Unlock()

	pack, offset := this.getPackForUpdateUnsafe(number)
	pack.BalanceSub(offset, amount, index)
}

func (this *Accounter) BalanceAdd(number uint32, amount uint64, index uint32) {
	this.lock.Lock()
	defer this.lock.Unlock()

	pack, offset := this.getPackForUpdateUnsafe(number)
	pack.BalanceAdd(offset, amount, index)
}

func (this *Accounter) KeyChange(number uint32, key *crypto.Public, index uint32, fee uint64) {
	this.lock.Lock()
	defer this.lock.Unlock()

	pack, offset := this.getPackForUpdateUnsafe(number)
	pack.KeyChange(offset, key, index, fee)
}

func (a *Accounter) Marshal() ([]byte, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()

	packs := make([]*PackPod, a.getHeightUnsafe())
	for each := range packs {
		pack := a.getPackUnsafe(uint32(each))
		packs[each] = pack.Pod()
	}

	pod := &AccounterPod{
		Packs: packs,
	}
	return pod.MarshalBinary()
}

func (a *Accounter) Unmarshal(data []byte) (int, error) {
	pod := AccounterPod{}
	size, err := pod.Unmarshal(data)
	if err != nil {
		return 0, err
	}

	a.packs = newPacksMap()
	for each := range pod.Packs {
		p := PackBase{}
		p.FromPod(*pod.Packs[each])
		a.appendPackUnsafe(&p)
	}
	a.Merge()

	return size, nil
}

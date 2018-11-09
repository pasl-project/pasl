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

package safebox

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/pasl-project/pasl/accounter"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/utils"
)

type Safebox struct {
	accounter *accounter.Accounter
	fork      Fork
	lock      sync.RWMutex
}

func NewSafebox(accounter *accounter.Accounter) *Safebox {
	height, SafeboxHash := accounter.GetState()
	return &Safebox{
		accounter: accounter,
		fork:      GetActiveFork(height, SafeboxHash),
	}
}

func (this *Safebox) getStateUnsafe() (uint32, []byte) {
	return this.accounter.GetState()
}

func (this *Safebox) GetState() (uint32, []byte) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getStateUnsafe()
}

func (this *Safebox) GetFork() Fork {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.fork
}

func (this *Safebox) GetForkByHeight(height uint32, prevSafeboxHash []byte) Fork {
	return GetActiveFork(height, prevSafeboxHash)
}

func (this *Safebox) SetFork(fork Fork) {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.fork = fork
}

func (this *Safebox) Validate(operation *tx.Tx) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	// TODO: code duplicaion
	height, _ := this.getStateUnsafe()
	_, err := operation.Validate(func(number uint32) *accounter.Account {
		accountPack := number / uint32(defaults.AccountsPerBlock)
		if accountPack+defaults.MaturationHeight < height {
			return this.accounter.GetAccount(number)
		}
		return nil
	})
	return err
}

func (this *Safebox) validateSignatures(operations *[]tx.Tx) error {
	wg := &sync.WaitGroup{}
	invalid := uint32(0)
	wg.Add(len(*operations))
	for index, _ := range *operations {
		go func(index int) {
			defer wg.Done()
			if (*operations)[index].ValidateSignature() != nil {
				atomic.StoreUint32(&invalid, 1)
			}
		}(index)
	}
	wg.Wait()
	if invalid != 0 {
		return errors.New("At least one of the txes didn't pass signature check")
	}
	return nil
}

func (this *Safebox) ProcessOperations(miner *crypto.Public, timestamp uint32, operations []tx.Tx) (*Safebox, []*accounter.Account, map[*accounter.Account]*tx.Tx, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	newSafebox := &Safebox{
		accounter: this.accounter.Copy(),
		fork:      this.fork,
	}

	updatedAccounts := make([]*accounter.Account, 0)

	newAccounts, newIndex := newSafebox.accounter.NewPack(miner, timestamp)
	newAccounts[0].Balance = getReward(newIndex)
	for _, it := range operations {
		newAccounts[0].Balance += it.GetFee()
	}
	updatedAccounts = append(updatedAccounts, newAccounts...)

	height, _ := this.getStateUnsafe()
	getMaturedAccountUnsafe := func(number uint32) *accounter.Account {
		accountPack := number / uint32(defaults.AccountsPerBlock)
		if accountPack+defaults.MaturationHeight < height {
			return this.accounter.GetAccount(number)
		}
		return nil
	}

	if err := this.validateSignatures(&operations); err != nil {
		return nil, nil, nil, err
	}

	affectedByTxes := make(map[*accounter.Account]*tx.Tx)
	for index, _ := range operations {
		context, err := operations[index].Validate(getMaturedAccountUnsafe)
		if err != nil {
			return nil, nil, nil, err
		}
		accountsAffected, err := operations[index].Apply(height, context)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, number := range accountsAffected {
			this.accounter.MarkAccountDirty(number)
			account := this.accounter.GetAccount(number)
			affectedByTxes[account] = &operations[index]
			updatedAccounts = append(updatedAccounts, account)
		}
	}

	return newSafebox, updatedAccounts, affectedByTxes, nil
}

func (this *Safebox) GetLastTimestamps(count uint32) (timestamps []uint32) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	timestamps = make([]uint32, 0, count)

	height, _ := this.accounter.GetState()
	var i uint32 = 0
	for ; i < count && height > 0; i++ {
		if account := this.accounter.GetAccount(height*uint32(defaults.AccountsPerBlock) - 1); account != nil {
			timestamps = append(timestamps, account.GetTimestamp())
		} else {
			break
		}
		height--
	}

	return timestamps
}

func (this *Safebox) GetAccount(number uint32) *accounter.Account {
	this.lock.RLock()
	defer this.lock.RUnlock()

	height, _ := this.getStateUnsafe()
	accountPack := number / uint32(defaults.AccountsPerBlock)
	if accountPack+defaults.MaturationHeight < height {
		account := *this.accounter.GetAccount(number)
		return &account
	}
	return nil
}

func getReward(index uint32) uint64 {
	magnitude := uint64(index / defaults.RewardDecreaseBlocks)
	reward := defaults.GenesisReward
	if magnitude > 0 {
		reward = reward / (magnitude * 2)
	}
	return utils.MaxUint64(reward, defaults.MinReward)
}

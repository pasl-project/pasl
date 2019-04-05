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
	"math/big"
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

type SafeboxBase interface {
	ToBlob() []byte
	GetHeight() uint32
	GetState() (height uint32, safeboxHash []byte, cumulativeDifficulty *big.Int)
	GetFork() Fork
	GetForkByHeight(height uint32, prevSafeboxHash []byte) Fork
	SetFork(fork Fork)
	Validate(operation tx.CommonOperation) error
	Merge()
	Rollback()
	GetUpdatedPacks() []uint32
	ProcessOperations(miner *crypto.Public, timestamp uint32, operations []tx.CommonOperation, difficulty *big.Int) (map[*accounter.Account]map[uint32]uint32, error)
	GetLastTimestamps(count uint32) (timestamps []uint32)
	GetHashrate(blockIndex, blocksCount uint32) uint64
	GetAccount(number uint32) *accounter.Account
	GetAccountPackSerialized(index uint32) ([]byte, error)
	SerializeAccounter() ([]byte, error)
}

func NewSafebox(accounter *accounter.Accounter) SafeboxBase {
	height, SafeboxHash, _ := accounter.GetState()
	return &Safebox{
		accounter: accounter,
		fork:      GetActiveFork(height, SafeboxHash),
	}
}

func (this *Safebox) ToBlob() []byte {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.accounter.ToBlob()
}

func (s *Safebox) GetHeight() uint32 {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.accounter.GetHeight()
}

func (this *Safebox) GetState() (height uint32, safeboxHash []byte, cumulativeDifficulty *big.Int) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.accounter.GetState()
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

func (this *Safebox) Validate(operation tx.CommonOperation) error {
	this.lock.RLock()
	defer this.lock.RUnlock()

	// TODO: code duplicaion
	height := this.accounter.GetHeight()
	_, err := tx.Validate(operation, func(number uint32) *accounter.Account {
		accountPack := number / uint32(defaults.AccountsPerBlock)
		if accountPack+defaults.MaturationHeight < height {
			return this.accounter.GetAccount(number)
		}
		return nil
	})
	return err
}

func (this *Safebox) validateSignatures(operations []tx.CommonOperation) error {
	wg := &sync.WaitGroup{}
	invalid := uint32(0)
	wg.Add(len(operations))
	for index := range operations {
		go func(index int) {
			defer wg.Done()
			if tx.ValidateSignature(operations[index]) != nil {
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

func (s *Safebox) Merge() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.accounter.Merge()
}

func (s *Safebox) Rollback() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.rollbackUnsafe()
}

func (s *Safebox) rollbackUnsafe() {
	s.accounter.Rollback()
}

func (s *Safebox) GetUpdatedPacks() []uint32 {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.accounter.GetUpdatedPacks()
}

func (this *Safebox) processOperationsUnsafe(miner *crypto.Public, timestamp uint32, operations []tx.CommonOperation, difficulty *big.Int) (map[*accounter.Account]map[uint32]uint32, error) {
	if err := this.validateSignatures(operations); err != nil {
		return nil, err
	}

	blockIndex := this.accounter.GetHeight()
	getMaturedAccountUnsafe := func(number uint32) *accounter.Account {
		accountPack := number / uint32(defaults.AccountsPerBlock)
		if accountPack+defaults.MaturationHeight <= blockIndex {
			return this.accounter.GetAccount(number)
		}
		return nil
	}

	if miner != nil {
		reward := getReward(blockIndex + 1)
		for index := range operations {
			reward += operations[index].GetFee()
		}
		this.accounter.NewPack(miner, reward, timestamp, difficulty)
	}

	affectedByTxes := make(map[*accounter.Account]map[uint32]uint32)
	for index := range operations {
		context, err := tx.Validate(operations[index], getMaturedAccountUnsafe)
		if err != nil {
			return nil, err
		}
		accountsAffected, err := operations[index].Apply(blockIndex, context, this.accounter)
		if err != nil {
			return nil, err
		}
		for _, number := range accountsAffected {
			account := this.accounter.GetAccount(number)
			if _, ok := affectedByTxes[account]; !ok {
				affectedByTxes[account] = make(map[uint32]uint32)
			}
			affectedByTxes[account][uint32(index)] = account.GetOperationsTotal()
		}
	}

	return affectedByTxes, nil
}

func (s *Safebox) ProcessOperations(miner *crypto.Public, timestamp uint32, operations []tx.CommonOperation, difficulty *big.Int) (map[*accounter.Account]map[uint32]uint32, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	affectedByTxes, err := s.processOperationsUnsafe(miner, timestamp, operations, difficulty)
	if err != nil {
		s.rollbackUnsafe()
	}
	return affectedByTxes, err
}

func (this *Safebox) GetLastTimestamps(count uint32) (timestamps []uint32) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	timestamps = make([]uint32, 0, count)

	height := this.accounter.GetHeight()
	for i := uint32(0); i < count && height > 0; i++ {
		account := this.accounter.GetAccount(height*uint32(defaults.AccountsPerBlock) - 1)
		timestamps = append(timestamps, account.GetTimestamp())
		height--
	}

	return timestamps
}

func (this *Safebox) GetHashrate(blockIndex, blocksCount uint32) uint64 {
	this.lock.RLock()
	defer this.lock.RUnlock()

	height := this.accounter.GetHeight()
	if blockIndex >= height {
		return 0
	}
	difficulty, timestamp := this.accounter.GetCumulativeDifficultyAndTimestamp(blockIndex)
	prevDifficulty, prevTimestamp := this.accounter.GetCumulativeDifficultyAndTimestamp(blockIndex - utils.MinUint32(blockIndex, blocksCount))
	difficulty.Sub(difficulty, prevDifficulty)
	elapsed := int64(timestamp - prevTimestamp)
	if elapsed == 0 {
		return 0
	}
	difficulty.Div(difficulty, big.NewInt(elapsed))
	return difficulty.Uint64()
}

func (this *Safebox) GetAccount(number uint32) *accounter.Account {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.accounter.GetAccount(number)
}

func getReward(index uint32) uint64 {
	magnitude := uint64(index / defaults.RewardDecreaseBlocks)
	reward := defaults.GenesisReward
	if magnitude > 0 {
		reward = reward / (magnitude * 2)
	}
	return utils.MaxUint64(reward, defaults.MinReward)
}

func (this *Safebox) GetAccountPackSerialized(index uint32) ([]byte, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.accounter.GetAccountPackSerialized(index)
}

func (this *Safebox) SerializeAccounter() ([]byte, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.accounter.Marshal()
}

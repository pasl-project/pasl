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

package blockchain

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pasl-project/pasl/storage"

	"github.com/pasl-project/pasl/accounter"
	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/utils"
)

type Blockchain struct {
	txPool  sync.Map
	storage *storage.Storage
	safebox *safebox.Safebox
	lock    sync.RWMutex
	target  common.TargetBase
}

func NewBlockchain(storage *storage.Storage) (*Blockchain, error) {
	accounter := accounter.NewAccounter()
	var topBlock *safebox.BlockMetadata
	var err error
	if topBlock, err = load(storage, accounter); err != nil {
		utils.Tracef("Error loading blockchain: %s", err.Error())
		return nil, err
	}

	height, safeboxHash := accounter.GetState()
	utils.Tracef("Blockchain loaded, height %d safeboxHash %s", height, hex.EncodeToString(safeboxHash))

	getPrevTarget := func() common.TargetBase {
		if topBlock != nil {
			return common.NewTarget(topBlock.Target)
		}
		return common.NewTarget(defaults.MinTarget)
	}

	safebox := safebox.NewSafebox(accounter)
	target := safebox.GetFork().GetNextTarget(getPrevTarget(), safebox.GetLastTimestamps)

	return &Blockchain{
		storage: storage,
		safebox: safebox,
		target:  common.NewTarget(target),
	}, nil
}

func load(storage *storage.Storage, accounterInstance *accounter.Accounter) (topBlock *safebox.BlockMetadata, err error) {
	var index uint32 = 0
	var i uint32 = 0
	accounts := make([]*accounter.Account, defaults.AccountsPerBlock)

	height, err := storage.Load(func(number uint32, data []byte) error {
		var account accounter.Account
		if err := utils.Deserialize(&account, bytes.NewBuffer(data)); err != nil {
			return err
		}
		accounts[i] = &account
		i += 1
		if i == defaults.AccountsPerBlock {
			i = 0
			accounterInstance.AppendPack(accounter.NewPackWithAccounts(index, accounts))
			index += 1
		}
		return nil
	})
	if err != nil {
		return
	}
	if i != 0 {
		err = errors.New("Accounts count doesn't fit the blockchain requirement")
		return
	}
	if height != index {
		err = errors.New("Non-consistent accounts count compared to the blockchain height")
		return
	}
	if height == 0 {
		return
	}

	var serialized []byte
	serialized, err = storage.GetBlock(height - 1)
	if err != nil {
		return
	}
	var meta safebox.BlockMetadata
	if err = utils.Deserialize(&meta, bytes.NewBuffer(serialized)); err != nil {
		return
	}

	return &meta, nil
}

func (this *Blockchain) AddBlock(meta *safebox.BlockMetadata, parentNotFound *bool) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	block, err := safebox.NewBlock(meta)
	if err != nil {
		return err
	}

	// TODO: block.Header.Time, implement NAT
	// TODO: check block hash for genesis block
	height, safeboxHash := this.safebox.GetState()
	if height != block.GetIndex() {
		return nil
	}
	if !bytes.Equal(safeboxHash, block.GetPrevSafeBoxHash()) {
		if parentNotFound != nil {
			*parentNotFound = true
		}
		return fmt.Errorf("Invalid block %d safeboxHash %s != %s expected", block.GetIndex(), hex.EncodeToString(block.GetPrevSafeBoxHash()), hex.EncodeToString(safeboxHash))
	}

	lastTimestamps := this.safebox.GetLastTimestamps(1)
	if len(lastTimestamps) != 0 && block.GetTimestamp() < lastTimestamps[0] {
		return errors.New("Invalid timestamp")
	}
	if err := this.safebox.GetFork().CheckBlock(this.target, block); err != nil {
		return errors.New("Invalid block: " + err.Error())
	}

	newSafebox, updatedAccounts, err := this.safebox.ProcessOperations(block.GetMiner(), block.GetTimestamp(), block.GetOperations())
	if err != nil {
		return err
	}

	newHeight, _ := newSafebox.GetState()
	if fork := safebox.TryActivateFork(newHeight, block.GetPrevSafeBoxHash()); fork != nil {
		this.target = block.GetTarget()
		newSafebox.SetFork(fork)
	}

	err = this.storage.Store(block.GetIndex(), utils.Serialize(meta), func(fn func(number uint32, data []byte) error) error {
		for _, account := range updatedAccounts {
			if err := fn(account.Number, utils.Serialize(account)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		utils.Tracef("Error storing blockchain state: %v", err)
		return err
	}

	this.txPoolCleanUpUnsafe(block.GetOperations())
	this.target.Set(newSafebox.GetFork().GetNextTarget(this.target, newSafebox.GetLastTimestamps))
	this.safebox = newSafebox
	return nil
}

func (this *Blockchain) AddBlockSerialized(block *safebox.SerializedBlock, parentNotFound *bool) error {
	return this.AddBlock(&safebox.BlockMetadata{
		Index:           block.Header.Index,
		Miner:           block.Header.Miner,
		Version:         block.Header.Version,
		Timestamp:       block.Header.Time,
		Target:          block.Header.Target,
		Nonce:           block.Header.Nonce,
		Payload:         block.Header.Payload,
		PrevSafeBoxHash: block.Header.PrevSafeboxHash,
		Operations:      block.Operations,
	}, parentNotFound)
}

func (this *Blockchain) AddOperation(operation *tx.Tx) (new bool, err error) {
	if err := this.safebox.Validate(operation); err != nil {
		return false, err
	}
	_, exists := this.txPool.LoadOrStore(operation.GetTxIdString(), *operation)
	return !exists, nil
}

func (this *Blockchain) txPoolCleanUpUnsafe(toRemove []tx.Tx) {
	this.txPool.Range(func(txId, value interface{}) bool {
		transaction := value.(tx.Tx)
		if err := this.safebox.Validate(&transaction); err != nil {
			toRemove = append(toRemove, transaction)
		}
		return true
	})
	for _, op := range toRemove {
		this.txPool.Delete(op.GetTxIdString())
	}
}

func (this *Blockchain) GetBlockTemplate(miner *crypto.Public, payload []byte) (template []byte, reservedOffset int, reservedSize int) {
	return this.safebox.GetFork().GetBlockHashingBlob(this.getPendingBlock(miner, payload))
}

func (this *Blockchain) GetPendingBlock() safebox.BlockBase {
	return this.getPendingBlock(nil, []byte(""))
}

func (this *Blockchain) getPendingBlock(miner *crypto.Public, payload []byte) safebox.BlockBase {
	var minerSerialized []byte
	if miner == nil {
		minerSerialized = utils.Serialize(crypto.NewKeyNil().Public)
	} else {
		minerSerialized = utils.Serialize(miner)
	}

	height, safeboxHash := this.safebox.GetState()

	operations := make([]tx.Tx, 0)
	this.txPool.Range(func(key, value interface{}) bool {
		operations = append(operations, value.(tx.Tx))
		return true
	})
	block, err := safebox.NewBlock(&safebox.BlockMetadata{
		Index: height,
		Miner: minerSerialized,
		Version: common.Version{
			Major: 1,
			Minor: 1,
		},
		Timestamp:       uint32(time.Now().Unix()),
		Target:          this.target.GetCompact(),
		Nonce:           0,
		Payload:         payload,
		PrevSafeBoxHash: safeboxHash,
		Operations:      operations,
	})
	if err != nil {
		utils.Tracef("Error %s", err.Error())
	}
	return block
}

func (this *Blockchain) GetBlock(index uint32) safebox.BlockBase {
	var serialized []byte
	serialized, err := this.storage.GetBlock(index)
	if err != nil {
		return nil
	}
	var meta safebox.BlockMetadata
	if err = utils.Deserialize(&meta, bytes.NewBuffer(serialized)); err != nil {
		return nil
	}

	block, err := safebox.NewBlock(&meta)
	if err != nil {
		return nil
	}

	return block
}

func (this *Blockchain) GetState() (height uint32, safeboxHash []byte) {
	return this.safebox.GetState()
}

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
	"math/big"
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
	storage storage.Storage
	safebox *safebox.Safebox
	lock    sync.RWMutex
	target  common.TargetBase
}

func NewBlockchain(storage storage.Storage) (*Blockchain, error) {
	accounter := accounter.NewAccounter()
	var topBlock *safebox.BlockMetadata
	var err error
	if topBlock, err = load(storage, accounter); err != nil {
		utils.Tracef("Error loading blockchain: %s", err.Error())
		return nil, err
	}

	height, safeboxHash, _ := accounter.GetState()
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

func load(storage storage.Storage, accounterInstance *accounter.Accounter) (topBlock *safebox.BlockMetadata, err error) {
	height, err := storage.Load(func(index uint32, data []byte) error {
		var pack accounter.PackBase
		if err := utils.Deserialize(&pack, bytes.NewBuffer(data)); err != nil {
			return err
		}
		accounterInstance.AppendPack(&pack)
		return nil
	})
	if err != nil {
		return
	}
	accounterHeight, _, _ := accounterInstance.GetState()
	if height != accounterHeight {
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

func (this *Blockchain) AddBlock(meta *safebox.BlockMetadata, parentNotFound *bool) (safebox.BlockBase, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	block, err := safebox.NewBlock(meta)
	if err != nil {
		return nil, err
	}

	// TODO: block.Header.Time, implement NAT
	// TODO: check block hash for genesis block
	height, safeboxHash, _ := this.safebox.GetState()
	if height != block.GetIndex() {
		return nil, nil
	}
	if !bytes.Equal(safeboxHash, block.GetPrevSafeBoxHash()) {
		if parentNotFound != nil {
			*parentNotFound = true
		}
		return nil, fmt.Errorf("Invalid block %d safeboxHash %s != %s expected", block.GetIndex(), hex.EncodeToString(block.GetPrevSafeBoxHash()), hex.EncodeToString(safeboxHash))
	}

	lastTimestamps := this.safebox.GetLastTimestamps(1)
	if len(lastTimestamps) != 0 && block.GetTimestamp() < lastTimestamps[0] {
		return nil, errors.New("Invalid timestamp")
	}
	if err := this.safebox.GetFork().CheckBlock(this.target, block); err != nil {
		return nil, errors.New("Invalid block: " + err.Error())
	}

	newSafebox, updatedPacks, affectedByTx, err := this.safebox.ProcessOperations(block.GetMiner(), block.GetTimestamp(), block.GetOperations(), block.GetTarget().GetDifficulty())
	if err != nil {
		return nil, err
	}

	newHeight, _, _ := newSafebox.GetState()
	if fork := safebox.TryActivateFork(newHeight, block.GetPrevSafeBoxHash()); fork != nil {
		this.target = block.GetTarget()
		newSafebox.SetFork(fork)
	}

	err = this.storage.Store(
		block.GetIndex(),
		utils.Serialize(meta),
		func(txes func(txRipemd160Hash [20]byte, txData []byte)) {
			for _, tx := range block.GetOperations() {
				serialized := bytes.NewBuffer([]byte(""))
				err := tx.Serialize(serialized)
				if err != nil {
					utils.Panicf("Failed to serailize tx")
				}
				txRipemd160Hash := [20]byte{}
				copy(txRipemd160Hash[:], tx.GetRipemd16Hash())
				txes(txRipemd160Hash, serialized.Bytes())
			}
		},
		func(accountOperations func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte)) {
			for account, tx := range affectedByTx {
				txRipemd160Hash := [20]byte{}
				copy(txRipemd160Hash[:], tx.GetRipemd16Hash())
				accountOperations(account.Number, account.OperationsTotal, txRipemd160Hash)
			}
		},
		func(fn func(number uint32, data []byte) error) error {
			for _, pack := range updatedPacks {
				if err := fn(pack.GetIndex(), utils.Serialize(pack)); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		utils.Tracef("Error storing blockchain state: %v", err)
		return nil, err
	}

	this.txPoolCleanUpUnsafe(block.GetOperations())
	this.target.Set(newSafebox.GetFork().GetNextTarget(this.target, newSafebox.GetLastTimestamps))
	this.safebox = newSafebox
	return block, nil
}

func (this *Blockchain) AddBlockSerialized(block *safebox.SerializedBlock, parentNotFound *bool) (safebox.BlockBase, error) {
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
	for index, _ := range toRemove {
		this.txPool.Delete(toRemove[index].GetTxIdString())
	}
}

func (this *Blockchain) GetBlockTemplate(miner *crypto.Public, payload []byte) (template []byte, reservedOffset int, reservedSize int) {
	return this.safebox.GetFork().GetBlockHashingBlob(this.getPendingBlock(miner, payload, uint32(time.Now().Unix())))
}

func (this *Blockchain) GetBlockPow(block safebox.BlockBase) []byte {
	fork := this.safebox.GetForkByHeight(block.GetIndex(), block.GetPrevSafeBoxHash())
	return fork.GetBlockPow(block)
}

func (this *Blockchain) SerializeBlockHeader(block safebox.BlockBase, willAppendOperations bool, nullPow bool) safebox.SerializedBlockHeader {
	var headerOnly uint8
	if willAppendOperations {
		headerOnly = 2
	} else {
		headerOnly = 3
	}
	var pow []byte
	if !nullPow {
		pow = this.GetBlockPow(block)
	} else {
		pow = make([]byte, 32)
	}
	return safebox.SerializedBlockHeader{
		HeaderOnly: headerOnly,
		Version: common.Version{
			Major: 1,
			Minor: 1,
		},
		Index:           block.GetIndex(),
		Miner:           utils.Serialize(block.GetMiner()),
		Reward:          block.GetReward(),
		Fee:             block.GetFee(),
		Time:            block.GetTimestamp(),
		Target:          block.GetTarget().GetCompact(),
		Nonce:           block.GetNonce(),
		Payload:         block.GetPayload(),
		PrevSafeboxHash: block.GetPrevSafeBoxHash(),
		OperationsHash:  block.GetOperationsHash(),
		Pow:             pow,
	}
}

func (this *Blockchain) SerializeBlock(block safebox.BlockBase) safebox.SerializedBlock {
	return safebox.SerializedBlock{
		Header:     this.SerializeBlockHeader(block, true, false),
		Operations: block.GetOperations(),
	}
}

func (this *Blockchain) GetPendingBlock(timestamp *uint32) safebox.BlockBase {
	var blockTimestamp uint32
	if timestamp == nil {
		blockTimestamp = uint32(time.Now().Unix())
	} else {
		blockTimestamp = *timestamp
	}
	return this.getPendingBlock(nil, []byte(""), blockTimestamp)
}

func (this *Blockchain) GetTxPool() []tx.Tx {
	operations := make([]tx.Tx, 0)
	this.txPool.Range(func(key, value interface{}) bool {
		operations = append(operations, value.(tx.Tx))
		return true
	})
	return operations
}

func (this *Blockchain) getPendingBlock(miner *crypto.Public, payload []byte, timestamp uint32) safebox.BlockBase {
	var minerSerialized []byte
	if miner == nil {
		minerSerialized = utils.Serialize(crypto.NewKeyNil().Public)
	} else {
		minerSerialized = utils.Serialize(miner)
	}

	height, safeboxHash, _ := this.safebox.GetState()

	operations := make([]tx.Tx, 0)
	this.txPool.Range(func(key, value interface{}) bool {
		tx := value.(tx.Tx)
		operations = append(operations, tx)
		return true
	})
	block, err := safebox.NewBlock(&safebox.BlockMetadata{
		Index: height,
		Miner: minerSerialized,
		Version: common.Version{
			Major: 1,
			Minor: 1,
		},
		Timestamp:       timestamp,
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

func (this *Blockchain) GetState() (height uint32, safeboxHash []byte, cumulativeDifficulty *big.Int) {
	return this.safebox.GetState()
}

func (this *Blockchain) GetHashrate(blockIndex, blocksCount uint32) uint64 {
	return this.safebox.GetHashrate(blockIndex, blocksCount)
}

func (this *Blockchain) GetAccount(number uint32) *accounter.Account {
	return this.safebox.GetAccount(number)
}

func (this *Blockchain) GetOperation(txRipemd160Hash [20]byte) *tx.Tx {
	serialized, err := this.storage.GetTx(txRipemd160Hash)
	if err != nil {
		return nil
	}

	var tx tx.Tx
	if err := tx.Deserialize(bytes.NewBuffer(serialized)); err != nil {
		utils.Tracef("Failed to deserialize tx")
		return nil
	}

	return &tx
}

func (this *Blockchain) GetAccountOperations(number uint32) (txesData map[uint32]*tx.Tx) {
	serializedTxes, err := this.storage.GetAccountTxesData(number)
	if err != nil {
		return nil
	}

	result := make(map[uint32]*tx.Tx)
	for operationId, serializedTx := range serializedTxes {
		result[operationId] = &tx.Tx{}
		if err := result[operationId].Deserialize(bytes.NewBuffer(serializedTx)); err != nil {
			utils.Tracef("Failed to deserialize tx")
			return nil
		}
	}

	return result
}

func (this *Blockchain) GetBlockOperations(index uint32) (txesData []tx.Tx) {
	block := this.GetBlock(index)
	if block == nil {
		utils.Tracef("Failed to get block %d", index)
		return nil
	}

	return block.GetOperations()
}

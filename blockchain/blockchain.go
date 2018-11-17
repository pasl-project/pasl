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

var (
	ErrParentNotFound = errors.New("Parent block not found")
)

type Blockchain struct {
	txPool  sync.Map
	storage storage.Storage
	safebox *safebox.Safebox
	lock    sync.RWMutex
	target  common.TargetBase
}

func NewBlockchain(s storage.Storage, height *uint32) (*Blockchain, error) {
	accounter := accounter.NewAccounter()
	var topBlock *safebox.BlockMetadata
	var err error

	restore := false
	if height == nil {
		if topBlock, err = load(s, accounter); err == storage.ErrSafeboxInconsistent {
			utils.Tracef("Restoring blockchain, will take a while")
			restore = true
		} else if err != nil {
			utils.Tracef("Error loading blockchain: %s", err.Error())
			return nil, err
		}
	}

	getPrevTarget := func() common.TargetBase {
		if topBlock != nil {
			return common.NewTarget(topBlock.Target)
		}
		return common.NewTarget(defaults.MinTarget)
	}

	safeboxInstance := safebox.NewSafebox(accounter)
	target := safeboxInstance.GetFork().GetNextTarget(getPrevTarget(), safeboxInstance.GetLastTimestamps)

	blockchain := &Blockchain{
		storage: s,
		safebox: safeboxInstance,
		target:  common.NewTarget(target),
	}

	if !restore && height == nil {
		return blockchain, nil
	}

	packs := make(map[uint32]struct{})
	if err = s.LoadBlocks(height, func(index uint32, data []byte) error {
		var blockMeta safebox.BlockMetadata
		if err := utils.Deserialize(&blockMeta, bytes.NewBuffer(data)); err != nil {
			return err
		}
		_, updatedPacks, err := blockchain.AddBlock(&blockMeta, false, false)
		for packIndex := range updatedPacks {
			packs[packIndex] = struct{}{}
		}
		return err
	}); err != nil {
		return nil, err
	}
	if restore {
		blockchain.FlushPacks(packs)
	}

	return blockchain, nil
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
		err = errors.New("Accounts count is not consistent with the blockchain height")
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

func (this *Blockchain) AddBlock(meta *safebox.BlockMetadata, store bool, syncing bool) (safebox.BlockBase, map[uint32]struct{}, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	block, err := safebox.NewBlock(meta)
	if err != nil {
		return nil, nil, err
	}

	// TODO: block.Header.Time, implement NAT
	// TODO: check block hash for genesis block
	height, safeboxHash, _ := this.safebox.GetState()
	if height != block.GetIndex() {
		return nil, make(map[uint32]struct{}), nil
	}
	if !bytes.Equal(safeboxHash, block.GetPrevSafeBoxHash()) {
		utils.Tracef("Invalid block %d safeboxHash %s != %s expected", block.GetIndex(), hex.EncodeToString(block.GetPrevSafeBoxHash()), hex.EncodeToString(safeboxHash))
		return nil, nil, ErrParentNotFound
	}

	lastTimestamps := this.safebox.GetLastTimestamps(1)
	if len(lastTimestamps) != 0 && block.GetTimestamp() < lastTimestamps[0] {
		return nil, nil, errors.New("Invalid timestamp")
	}
	if err := this.safebox.GetFork().CheckBlock(this.target, block); err != nil {
		return nil, nil, errors.New("Invalid block: " + err.Error())
	}

	newSafebox, updatedPacks, affectedByTx, err := this.safebox.ProcessOperations(block.GetMiner(), block.GetTimestamp(), block.GetOperations(), block.GetTarget().GetDifficulty())
	if err != nil {
		return nil, nil, err
	}

	newHeight, _, _ := newSafebox.GetState()
	if fork := safebox.TryActivateFork(newHeight, block.GetPrevSafeBoxHash()); fork != nil {
		this.target = block.GetTarget()
		newSafebox.SetFork(fork)
	}

	if store {
		err = this.storage.WithWritable(func(s storage.StorageWritable, ctx interface{}) error {
			if err = s.StoreBlock(ctx, block.GetIndex(), utils.Serialize(meta)); err != nil {
				return err
			}

			operations := block.GetOperations()
			txIDBytTxIndex := make(map[uint32]uint64)

			for index, _ := range operations {
				txIndexInsideBlock := uint32(index)
				tx := &operations[txIndexInsideBlock]
				txRipemd160Hash := [20]byte{}
				copy(txRipemd160Hash[:], tx.GetRipemd16Hash())
				txId, err := s.StoreTxHash(ctx, txRipemd160Hash, block.GetIndex(), txIndexInsideBlock)
				if err != nil {
					return err
				}
				txMetadata := tx.GetMetadata(txIndexInsideBlock, block.GetIndex(), block.GetTimestamp())
				if err = s.StoreTxMetadata(ctx, txId, utils.Serialize(txMetadata)); err != nil {
					return err
				}
				txIDBytTxIndex[txIndexInsideBlock] = txId
			}

			for account, txIndexInsideBlock := range affectedByTx {
				if err = s.StoreAccountOperation(ctx, account.GetNumber(), account.GetOperationsTotal(), txIDBytTxIndex[txIndexInsideBlock]); err != nil {
					return err
				}
			}

			if !syncing {
				for packIndex := range updatedPacks {
					if err = s.StoreAccountPack(ctx, packIndex, newSafebox.GetAccountPackSerialized(packIndex)); err != nil {
						return err
					}
				}
			} else {

			}

			return nil
		})

		if err != nil {
			utils.Tracef("Error storing blockchain state: %v", err)
			return nil, nil, err
		}
	}

	this.txPoolCleanUpUnsafe(block.GetOperations())
	this.target.Set(newSafebox.GetFork().GetNextTarget(this.target, newSafebox.GetLastTimestamps))
	this.safebox = newSafebox
	return block, updatedPacks, nil
}

func (this *Blockchain) AddBlockSerialized(block *safebox.SerializedBlock, syncing bool) (safebox.BlockBase, map[uint32]struct{}, error) {
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
	}, true, syncing)
}

func (this *Blockchain) FlushPacks(updatedPacks map[uint32]struct{}) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	return this.storage.WithWritable(func(s storage.StorageWritable, ctx interface{}) error {
		for packIndex := range updatedPacks {
			if err := s.StoreAccountPack(ctx, packIndex, this.safebox.GetAccountPackSerialized(packIndex)); err != nil {
				return err
			}
		}
		return nil
	})
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

func (this *Blockchain) TxPoolForEach(fn func(meta *tx.TxMetadata, tx *tx.Tx) bool) {
	i := uint32(0)
	time := uint32(time.Now().Unix())
	this.txPool.Range(func(key, value interface{}) bool {
		tx := value.(tx.Tx)
		meta := tx.GetMetadata(i, 0, time)
		i += 1
		return fn(&meta, &tx)
	})
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

func (this *Blockchain) GetOperation(txRipemd160Hash [20]byte) (*tx.TxMetadata, *tx.Tx, error) {
	metadataSerialized, err := this.storage.GetTxMetadata(txRipemd160Hash)
	if err != nil {
		return nil, nil, err
	}

	return tx.TxFromMetadata(metadataSerialized)
}

func (this *Blockchain) AccountOperationsForEach(number uint32, fn func(operationId uint32, meta *tx.TxMetadata, tx *tx.Tx) bool) error {
	serializedTxes, err := this.storage.GetAccountTxesData(number)
	if err != nil {
		return err
	}

	for operationId, _ := range serializedTxes {
		if meta, tx, err := tx.TxFromMetadata(serializedTxes[operationId]); err != nil {
			if !fn(operationId, meta, tx) {
				return nil
			}
		} else {
			return err
		}
	}

	return nil
}

func (this *Blockchain) BlockOperationsForEach(index uint32, fn func(meta *tx.TxMetadata, tx *tx.Tx) bool) error {
	block := this.GetBlock(index)
	if block == nil {
		return fmt.Errorf("Failed to get block %d", index)
	}

	operations := block.GetOperations()
	for index, _ := range operations {
		meta := operations[index].GetMetadata(uint32(index), block.GetIndex(), block.GetTimestamp())
		if !fn(&meta, &operations[index]) {
			return nil
		}
	}
	return nil
}

func (this *Blockchain) ExportSafebox() []byte {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.safebox.ToBlob()
}

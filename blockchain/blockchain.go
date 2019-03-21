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

	"github.com/pasl-project/pasl/accounter"
	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/storage"
	"github.com/pasl-project/pasl/utils"
)

var (
	ErrInvalidOrder    = errors.New("Unexpected block index")
	ErrFutureTimestamp = errors.New("Block time is too far in the future")
	ErrParentNotFound  = errors.New("Parent block not found")
)

type Blockchain struct {
	txPool  sync.Map
	storage storage.Storage
	safebox *safebox.Safebox
	lock    sync.RWMutex
	target  common.TargetBase
}

type blockInfo struct {
	meta         *safebox.BlockMetadata
	affectedByTx map[*accounter.Account]map[uint32]uint32
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

	blockchain := newBlockchain(s, accounter, getPrevTarget())

	if !restore && height == nil {
		return blockchain, nil
	}

	packs := make(map[uint32]struct{})
	if err = s.LoadBlocks(height, func(index uint32, data []byte) error {
		var blockMeta safebox.BlockMetadata
		if err := utils.Deserialize(&blockMeta, bytes.NewBuffer(data)); err != nil {
			return err
		}
		block, err := safebox.NewBlock(&blockMeta)
		if err != nil {
			return err
		}
		_, _, updatedPacks, _, err := blockchain.AddBlock(block)
		for packIndex := range updatedPacks {
			packs[packIndex] = struct{}{}
		}
		return err
	}); err != nil {
		return nil, err
	}
	if restore {
		blockchain.flushPacks(packs)
	}

	return blockchain, nil
}

func newBlockchain(s storage.Storage, accounter *accounter.Accounter, target common.TargetBase) *Blockchain {
	safeboxInstance := safebox.NewSafebox(accounter)
	nextTarget := safeboxInstance.GetFork().GetNextTarget(target, safeboxInstance.GetLastTimestamps)

	blockchain := &Blockchain{
		storage: s,
		safebox: safeboxInstance,
		target:  common.NewTarget(nextTarget),
	}

	return blockchain
}

func load(storage storage.Storage, accounterInstance *accounter.Accounter) (topBlock *safebox.BlockMetadata, err error) {
	height, err := storage.Load(func(index uint32, data []byte) error {
		var pack accounter.PackBase
		if err := utils.Deserialize(&pack, bytes.NewBuffer(data)); err != nil {
			return err
		}
		accounterInstance.AppendPack(pack)
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

func (this *Blockchain) AddBlock(block safebox.BlockBase) (*safebox.Safebox, common.TargetBase, map[uint32]struct{}, map[*accounter.Account]map[uint32]uint32, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	if block.GetTimestamp() > uint32(time.Now().Unix())+defaults.MaxBlockTimeOffset {
		return nil, nil, nil, nil, ErrFutureTimestamp
	}

	// TODO: check block hash for genesis block
	height, safeboxHash, _ := this.safebox.GetState()
	if height != block.GetIndex() {
		return nil, nil, nil, nil, ErrInvalidOrder
	}
	if !bytes.Equal(safeboxHash, block.GetPrevSafeBoxHash()) {
		utils.Tracef("Invalid block %d safeboxHash %s != %s expected", block.GetIndex(), hex.EncodeToString(block.GetPrevSafeBoxHash()), hex.EncodeToString(safeboxHash))
		return nil, nil, nil, nil, ErrParentNotFound
	}

	lastTimestamps := this.safebox.GetLastTimestamps(1)
	if len(lastTimestamps) != 0 && block.GetTimestamp() < lastTimestamps[0] {
		return nil, nil, nil, nil, errors.New("Invalid timestamp")
	}
	if err := this.safebox.GetFork().CheckBlock(this.target, block); err != nil {
		return nil, nil, nil, nil, errors.New("Invalid block: " + err.Error())
	}

	newSafebox, updatedPacks, affectedByTx, err := this.safebox.ProcessOperations(block.GetMiner(), block.GetTimestamp(), block.GetOperations(), block.GetTarget().GetDifficulty())
	if err != nil {
		return nil, nil, nil, nil, err
	}

	newHeight, _, _ := newSafebox.GetState()
	newTarget := this.target
	if fork := safebox.TryActivateFork(newHeight, block.GetPrevSafeBoxHash()); fork != nil {
		newTarget = block.GetTarget()
		newSafebox.SetFork(fork)
	}
	newTarget = common.NewTarget(newSafebox.GetFork().GetNextTarget(newTarget, newSafebox.GetLastTimestamps))

	return newSafebox, newTarget, updatedPacks, affectedByTx, nil
}

func (this *Blockchain) ProcessNewBlock(blockSerialized *safebox.SerializedBlock) error {
	meta := &safebox.BlockMetadata{
		Index:           blockSerialized.Header.Index,
		Miner:           blockSerialized.Header.Miner,
		Version:         blockSerialized.Header.Version,
		Timestamp:       blockSerialized.Header.Time,
		Target:          blockSerialized.Header.Target,
		Nonce:           blockSerialized.Header.Nonce,
		Payload:         blockSerialized.Header.Payload,
		PrevSafeBoxHash: blockSerialized.Header.PrevSafeboxHash,
		Operations:      blockSerialized.Operations,
	}
	block, err := safebox.NewBlock(meta)
	if err != nil {
		return err
	}

	newSafebox, newTarget, updatedPacks, affectedByTx, err := this.AddBlock(block)
	if err != nil {
		return err
	}

	err = this.storage.WithWritable(func(s storage.StorageWritable, ctx interface{}) error {
		for packIndex := range updatedPacks {
			if err := s.StoreAccountPack(ctx, packIndex, newSafebox.GetAccountPackSerialized(packIndex)); err != nil {
				return err
			}
		}

		if err = s.StoreBlock(ctx, block.GetIndex(), utils.Serialize(meta)); err != nil {
			return err
		}

		operations := block.GetOperations()
		txIDBytTxIndex := make(map[uint32]uint64)

		for index := range operations {
			txIndexInsideBlock := uint32(index)
			tx := &operations[txIndexInsideBlock]
			txRipemd160Hash := [20]byte{}
			copy(txRipemd160Hash[:], tx.GetRipemd16Hash())
			txID, err := s.StoreTxHash(ctx, txRipemd160Hash, block.GetIndex(), txIndexInsideBlock)
			if err != nil {
				return err
			}
			txMetadata := tx.GetMetadata(txIndexInsideBlock, block.GetIndex(), block.GetTimestamp())
			if err = s.StoreTxMetadata(ctx, txID, utils.Serialize(txMetadata)); err != nil {
				return err
			}
			txIDBytTxIndex[txIndexInsideBlock] = txID
		}

		for account, txIndexToAccountTxIndex := range affectedByTx {
			for txIndexInsideBlock, accountTxIndex := range txIndexToAccountTxIndex {
				if err = s.StoreAccountOperation(ctx, account.GetNumber(), accountTxIndex, txIDBytTxIndex[txIndexInsideBlock]); err != nil {
					return err
				}
			}
		}

		for packIndex := range updatedPacks {
			if err = s.StoreAccountPack(ctx, packIndex, newSafebox.GetAccountPackSerialized(packIndex)); err != nil {
				return err
			}
		}

		if block.GetIndex()%defaults.MaxAltChainLength == 0 {
			if buffer := newSafebox.SerializeAccounter(); buffer != nil {
				if err = s.StoreSnapshot(ctx, block.GetIndex()%(defaults.MaxAltChainLength*2)/defaults.MaxAltChainLength, buffer); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("failed to serialize accounter")
			}
		}

		return nil
	})

	if err != nil {
		utils.Tracef("Error storing blockchain state: %v", err)
		return err
	}

	this.lock.Lock()
	defer this.lock.Unlock()

	this.txPoolCleanUpUnsafe(block.GetOperations())
	this.safebox = newSafebox
	this.target = newTarget

	return nil
}

func (this *Blockchain) AddAlternateChain(blocks []safebox.SerializedBlock) error {
	{
		header := blocks[0].Header
		mainBlock := this.GetBlock(header.Index)
		if !bytes.Equal(mainBlock.GetPrevSafeBoxHash(), header.PrevSafeboxHash) {
			return fmt.Errorf("alternate chain parent not found")
		}
	}

	snapshot := accounter.NewAccounter()
	snapshotNumber := (blocks[0].Header.Index % (defaults.MaxAltChainLength * 2)) / defaults.MaxAltChainLength
	if buffer := this.storage.LoadSnapshot(snapshotNumber); buffer != nil {
		if err := snapshot.Deserialize(bytes.NewBuffer(buffer)); err != nil {
			return errors.New("failed to deserialize snapshot")
		}
	} else {
		return errors.New("failed to load snapshot")
	}

	snapshotHeight, _, _ := snapshot.GetState()
	mainBlock := this.GetBlock(snapshotHeight - 1)
	newBlockchain := newBlockchain(this.storage, snapshot, mainBlock.GetTarget())

	updatedPacksTotal := make(map[uint32]struct{})
	affectedByBlocks := make(map[safebox.BlockBase]blockInfo)

	mainBlocks := make([]safebox.SerializedBlock, 0)
	for index := snapshotHeight; index < blocks[0].Header.Index; index++ {
		mainBlock := this.GetBlock(index)
		mainBlocks = append(mainBlocks, this.SerializeBlock(mainBlock))
	}

	blocks = append(mainBlocks, blocks...)
	for index := range blocks {
		meta := &safebox.BlockMetadata{
			Index:           blocks[index].Header.Index,
			Miner:           blocks[index].Header.Miner,
			Version:         blocks[index].Header.Version,
			Timestamp:       blocks[index].Header.Time,
			Target:          blocks[index].Header.Target,
			Nonce:           blocks[index].Header.Nonce,
			Payload:         blocks[index].Header.Payload,
			PrevSafeBoxHash: blocks[index].Header.PrevSafeboxHash,
			Operations:      blocks[index].Operations,
		}
		block, err := safebox.NewBlock(meta)
		if err != nil {
			return err
		}

		newSafebox, newTarget, updatedPacks, affectedByTx, err := newBlockchain.AddBlock(block)
		if err != nil {
			return fmt.Errorf("failed to apply alternate chain: %v", err)
		}
		newBlockchain.safebox = newSafebox
		newBlockchain.target = newTarget
		for pack := range updatedPacks {
			updatedPacksTotal[pack] = struct{}{}
		}
		affectedByBlocks[block] = blockInfo{
			meta:         meta,
			affectedByTx: affectedByTx,
		}
	}

	this.lock.Lock()
	defer this.lock.Unlock()

	_, _, cumulativeDifficulty := newBlockchain.GetState()
	_, _, currentCumulativeDifficulty := this.GetState()
	if currentCumulativeDifficulty.Cmp(cumulativeDifficulty) >= 0 {
		return fmt.Errorf("cumulative difficulty: main chain %s >= %s alt chain", currentCumulativeDifficulty.String(), cumulativeDifficulty.String())
	}

	// TODO: code duplication
	err := newBlockchain.storage.WithWritable(func(s storage.StorageWritable, ctx interface{}) error {
		for block, blockInfo := range affectedByBlocks {
			if err := s.StoreBlock(ctx, block.GetIndex(), utils.Serialize(blockInfo.meta)); err != nil {
				return err
			}

			operations := block.GetOperations()
			txIDBytTxIndex := make(map[uint32]uint64)

			for index := range operations {
				txIndexInsideBlock := uint32(index)
				tx := &operations[txIndexInsideBlock]
				txRipemd160Hash := [20]byte{}
				copy(txRipemd160Hash[:], tx.GetRipemd16Hash())
				txID, err := s.StoreTxHash(ctx, txRipemd160Hash, block.GetIndex(), txIndexInsideBlock)
				if err != nil {
					return err
				}
				txMetadata := tx.GetMetadata(txIndexInsideBlock, block.GetIndex(), block.GetTimestamp())
				if err = s.StoreTxMetadata(ctx, txID, utils.Serialize(txMetadata)); err != nil {
					return err
				}
				txIDBytTxIndex[txIndexInsideBlock] = txID
			}

			for account, txIndexToAccountTxIndex := range blockInfo.affectedByTx {
				for txIndexInsideBlock, accountTxIndex := range txIndexToAccountTxIndex {
					if err := s.StoreAccountOperation(ctx, account.GetNumber(), accountTxIndex, txIDBytTxIndex[txIndexInsideBlock]); err != nil {
						return err
					}
				}
			}

			for packIndex := range updatedPacksTotal {
				if err := s.StoreAccountPack(ctx, packIndex, newBlockchain.safebox.GetAccountPackSerialized(packIndex)); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		utils.Tracef("Error storing blockchain state: %v", err)
		return err
	}

	// TODO: drop orphaned blocks and txes from storage
	// TODO: invalidate mem pool txes

	this.safebox = newBlockchain.safebox
	this.target = newBlockchain.target

	return nil
}

func (this *Blockchain) flushPacks(updatedPacks map[uint32]struct{}) error {
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

func (this *Blockchain) TxPoolAddOperation(operation *tx.Tx) (new bool, err error) {
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

func (this *Blockchain) GetBlockTemplate(miner *crypto.Public, payload []byte, time uint32) (template []byte, reservedOffset int, reservedSize int) {
	return this.safebox.GetFork().GetBlockHashingBlob(this.getPendingBlock(miner, payload, time))
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

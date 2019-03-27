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
	"sort"
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
	txPool              sync.Map
	storage             storage.Storage
	safebox             *safebox.Safebox
	lock                sync.RWMutex
	target              common.TargetBase
	blocksSinceSnapshot uint32
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

	if err = s.LoadBlocks(height, func(index uint32, data []byte) error {
		var blockMeta safebox.BlockMetadata
		if err := utils.Deserialize(&blockMeta, bytes.NewBuffer(data)); err != nil {
			return err
		}
		block, err := safebox.NewBlock(&blockMeta)
		if err != nil {
			return err
		}
		newTarget, _, err := blockchain.addBlock(blockchain.target, block)
		blockchain.target = newTarget
		return err
	}); err != nil {
		return nil, err
	}
	blockchain.safebox.Merge()
	if restore {
		blockchain.flushPacks(blockchain.safebox.GetUpdatedPacks())
	}

	return blockchain, nil
}

func newBlockchain(s storage.Storage, accounter *accounter.Accounter, target common.TargetBase) *Blockchain {
	safeboxInstance := safebox.NewSafebox(*accounter)
	nextTarget := safeboxInstance.GetFork().GetNextTarget(target, safeboxInstance.GetLastTimestamps)

	blockchain := &Blockchain{
		storage:             s,
		safebox:             safeboxInstance,
		target:              common.NewTarget(nextTarget),
		blocksSinceSnapshot: 0,
	}

	return blockchain
}

func load(storage storage.Storage, accounterInstance *accounter.Accounter) (topBlock *safebox.BlockMetadata, err error) {
	height, err := storage.Load(func(index uint32, data []byte) error {
		var pack accounter.PackBase
		_, err := pack.Unmarshal(data)
		if err != nil {
			return err
		}
		accounterInstance.AppendPack(&pack)
		return nil
	})
	if err != nil {
		return
	}
	accounterInstance.Merge()
	accounterHeight := accounterInstance.GetHeight()
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

func (b *Blockchain) addBlock(target common.TargetBase, block safebox.BlockBase) (common.TargetBase, map[*accounter.Account]map[uint32]uint32, error) {
	if block.GetTimestamp() > uint32(time.Now().Unix())+defaults.MaxBlockTimeOffset {
		return nil, nil, ErrFutureTimestamp
	}

	// TODO: check block hash for genesis block
	height, safeboxHash, _ := b.safebox.GetState()
	if height != block.GetIndex() {
		return nil, nil, ErrInvalidOrder
	}
	if !bytes.Equal(safeboxHash, block.GetPrevSafeBoxHash()) {
		utils.Tracef("Invalid block %d safeboxHash %s != %s expected", block.GetIndex(), hex.EncodeToString(block.GetPrevSafeBoxHash()), hex.EncodeToString(safeboxHash))
		return nil, nil, ErrParentNotFound
	}

	lastTimestamps := b.safebox.GetLastTimestamps(1)
	if len(lastTimestamps) != 0 && block.GetTimestamp() < lastTimestamps[0] {
		return nil, nil, errors.New("Invalid timestamp")
	}
	if err := b.safebox.GetFork().CheckBlock(target, block); err != nil {
		return nil, nil, errors.New("Invalid block: " + err.Error())
	}

	affectedByTx, err := b.safebox.ProcessOperations(block.GetMiner(), block.GetTimestamp(), block.GetOperations(), block.GetTarget().GetDifficulty())
	if err != nil {
		return nil, nil, err
	}

	newHeight := height + 1
	newTarget := target
	if fork := safebox.TryActivateFork(newHeight, block.GetPrevSafeBoxHash()); fork != nil {
		newTarget = block.GetTarget()
		b.safebox.SetFork(fork)
	}
	newTarget = common.NewTarget(b.safebox.GetFork().GetNextTarget(newTarget, b.safebox.GetLastTimestamps))

	return newTarget, affectedByTx, nil
}

func (this *Blockchain) processNewBlocksUnsafe(blocks []safebox.SerializedBlock, preSave *func(*safebox.Safebox, common.TargetBase) error) error {
	currentTarget := this.target
	affectedByBlocks := make(map[safebox.BlockBase]blockInfo)
	blocksProcessed := make([]safebox.BlockBase, 0, len(blocks))

	sort.Slice(blocks, func(i, j int) bool { return blocks[i].Header.Index < blocks[j].Header.Index })
	for _, blockSerialized := range blocks {
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
		blocksProcessed = append(blocksProcessed, block)

		newTarget, affectedByTx, err := this.addBlock(currentTarget, block)
		if err != nil {
			return err
		}
		currentTarget = newTarget

		affectedByBlocks[block] = blockInfo{
			meta:         meta,
			affectedByTx: affectedByTx,
		}
	}

	if preSave != nil {
		if err := (*preSave)(this.safebox, currentTarget); err != nil {
			return err
		}
	}

	updatedPacks := this.safebox.GetUpdatedPacks()
	err := this.storage.WithWritable(func(s storage.StorageWritable, ctx interface{}) error {
		for _, packIndex := range updatedPacks {
			data, err := this.safebox.GetAccountPackSerialized(packIndex)
			if err != nil {
				return err
			}
			if err := s.StoreAccountPack(ctx, packIndex, data); err != nil {
				return err
			}
		}

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
		}

		this.blocksSinceSnapshot += uint32(len(affectedByBlocks))
		if this.blocksSinceSnapshot > defaults.MaxAltChainLength/2 {
			height := this.safebox.GetHeight()

			snapshots := this.storage.ListSnapshots()
			sort.Slice(snapshots, func(i, j int) bool { return snapshots[i] > snapshots[j] })
			for index := 1; index < len(snapshots); index++ {
				s.DropSnapshot(ctx, snapshots[index])
			}

			buffer, err := this.safebox.SerializeAccounter()
			if err == nil {
				if err := s.StoreSnapshot(ctx, height, buffer); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("failed to serialize accounter: %v", err)
			}

			this.blocksSinceSnapshot = 0
		}

		return nil
	})

	if err != nil {
		utils.Tracef("Error storing blockchain state: %v", err)
		return err
	}

	for _, block := range blocksProcessed {
		this.txPoolCleanUpUnsafe(block.GetOperations())
	}

	this.target = currentTarget

	return nil
}

func (this *Blockchain) ProcessNewBlocks(blocks []safebox.SerializedBlock, preSave *func(*safebox.Safebox, common.TargetBase) error) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	if err := this.processNewBlocksUnsafe(blocks, preSave); err != nil {
		this.safebox.Rollback()
		return err
	}
	this.safebox.Merge()
	return nil
}

func (this *Blockchain) ProcessNewBlock(block safebox.SerializedBlock) error {
	return this.ProcessNewBlocks([]safebox.SerializedBlock{block}, nil)
}

func (this *Blockchain) LoadSnapshot(height uint32) (*accounter.Accounter, error) {
	buffer := this.storage.LoadSnapshot(height)
	if buffer == nil {
		return nil, fmt.Errorf("failed to load snapshot %d", height)
	}
	snapshot := accounter.NewAccounter()
	_, err := snapshot.Unmarshal(buffer)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize snapshot %d", height)
	}
	return snapshot, nil
}

func (this *Blockchain) LoadNearestSnapshot(targetHeight uint32) (*accounter.Accounter, error) {
	found := false
	height := uint32(0)
	for _, snapshotHeight := range this.storage.ListSnapshots() {
		if snapshotHeight > targetHeight {
			continue
		}
		if !found || snapshotHeight > height {
			found = true
			height = snapshotHeight
		}
	}

	if !found {
		return nil, fmt.Errorf("no matching snapshots")
	}

	return this.LoadSnapshot(height)
}

func (this *Blockchain) AddAlternateChain(blocks []safebox.SerializedBlock) error {
	if len(blocks) == 0 {
		return nil
	}

	{
		header := blocks[0].Header
		mainBlock := this.GetBlock(header.Index)
		if !bytes.Equal(mainBlock.GetPrevSafeBoxHash(), header.PrevSafeboxHash) {
			return fmt.Errorf("alternate chain parent not found")
		}
	}

	snapshot, err := this.LoadNearestSnapshot(blocks[0].Header.Index)
	if err != nil {
		return fmt.Errorf("failed to find nearest snapshot: %v", err)
	}

	snapshotHeight := snapshot.GetHeight()
	mainBlock := this.GetBlock(snapshotHeight - 1)

	newBlockchain := newBlockchain(this.storage, snapshot, mainBlock.GetTarget())
	currentTarget := newBlockchain.target
	for index := snapshotHeight; index < blocks[0].Header.Index; index++ {
		block := this.GetBlock(index)
		newTarget, _, err := this.addBlock(currentTarget, block)
		if err != nil {
			return err
		}
		currentTarget = newTarget
	}
	newBlockchain.target = currentTarget

	this.lock.Lock()
	defer this.lock.Unlock()

	cumulativeDifficultyCheck := func(altSafebox *safebox.Safebox, target common.TargetBase) error {
		_, _, cumulativeDifficulty := altSafebox.GetState()
		_, _, currentCumulativeDifficulty := this.GetState()
		if currentCumulativeDifficulty.Cmp(cumulativeDifficulty) >= 0 {
			return fmt.Errorf("cumulative difficulty: main chain %s >= %s alt chain", currentCumulativeDifficulty.String(), cumulativeDifficulty.String())
		}
		return nil
	}

	if err := newBlockchain.ProcessNewBlocks(blocks, &cumulativeDifficultyCheck); err != nil {
		utils.Tracef("rejected alt chain: %v", err)
		return err
	}

	// TODO: drop orphaned blocks and txes from storage
	// TODO: invalidate mem pool txes

	this.safebox = newBlockchain.safebox
	this.target = newBlockchain.target

	return nil
}

func (this *Blockchain) flushPacks(updatedPacks []uint32) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	return this.storage.WithWritable(func(s storage.StorageWritable, ctx interface{}) error {
		for _, packIndex := range updatedPacks {
			data, err := this.safebox.GetAccountPackSerialized(packIndex)
			if err != nil {
				return err
			}
			if err := s.StoreAccountPack(ctx, packIndex, data); err != nil {
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

func (b *Blockchain) GetTopBlock() safebox.BlockBase {
	b.lock.Lock()
	defer b.lock.Unlock()

	if height := b.GetHeight(); height > 0 {
		return b.GetBlock(height - 1)
	}
	return b.GetPendingBlock(nil)
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

func (this *Blockchain) GetHeight() uint32 {
	return this.safebox.GetHeight()
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

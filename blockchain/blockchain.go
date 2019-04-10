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

	"github.com/mantyr/iterator"
)

var (
	ErrInvalidOrder    = errors.New("Unexpected block index")
	ErrFutureTimestamp = errors.New("Block time is too far in the future")
	ErrParentNotFound  = errors.New("Parent block not found")
)

type NewSafeboxCallback func(accounter *accounter.Accounter) safebox.SafeboxBase

type Blockchain struct {
	txPool              *iterator.Items
	storage             storage.Storage
	safebox             safebox.SafeboxBase
	lock                sync.RWMutex
	target              common.TargetBase
	blocksSinceSnapshot uint32
	BlocksUpdates       chan safebox.SerializedBlock
	TxPoolUpdates       chan tx.CommonOperation
	newSafeboxCallback  NewSafeboxCallback
	prevSafeboxHash     []byte
}

type blockInfo struct {
	meta         *safebox.BlockMetadata
	affectedByTx map[*accounter.Account]map[uint32]uint32
}

func NewBlockchain(fn NewSafeboxCallback, s storage.Storage, height *uint32) (*Blockchain, error) {
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

	blockchain := newBlockchain(fn, s, accounter, getPrevTarget())

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

func newBlockchain(fn NewSafeboxCallback, s storage.Storage, accounter *accounter.Accounter, target common.TargetBase) *Blockchain {
	safeboxInstance := fn(accounter)
	nextTarget := safeboxInstance.GetFork().GetNextTarget(target, safeboxInstance.GetLastTimestamps)

	_, safeboxHash, _ := safeboxInstance.GetState()

	blockchain := &Blockchain{
		blocksSinceSnapshot: 0,
		safebox:             safeboxInstance,
		storage:             s,
		target:              common.NewTarget(nextTarget),
		txPool:              iterator.New(),
		BlocksUpdates:       make(chan safebox.SerializedBlock),
		TxPoolUpdates:       make(chan tx.CommonOperation),
		newSafeboxCallback:  fn,
		prevSafeboxHash:     make([]byte, len(safeboxHash)),
	}
	copy(blockchain.prevSafeboxHash, safeboxHash)

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

func (this *Blockchain) processNewBlocksUnsafe(blocks []safebox.SerializedBlock, preSave *func(safebox.SafeboxBase) error) error {
	currentTarget := this.target
	affectedByBlocks := make(map[safebox.BlockBase]blockInfo)
	blocksProcessed := make([]safebox.BlockBase, 0, len(blocks))

	var affectedByTx map[*accounter.Account]map[uint32]uint32
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

		currentTarget, affectedByTx, err = this.addBlock(currentTarget, block)
		if err != nil {
			return err
		}

		affectedByBlocks[block] = blockInfo{
			meta:         meta,
			affectedByTx: affectedByTx,
		}
	}

	if preSave != nil {
		if err := (*preSave)(this.safebox); err != nil {
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
				transaction := operations[txIndexInsideBlock]
				txRipemd160Hash := [20]byte{}
				copy(txRipemd160Hash[:], tx.GetRipemd16Hash(transaction))
				txID, err := s.StoreTxHash(ctx, txRipemd160Hash, block.GetIndex(), txIndexInsideBlock)
				if err != nil {
					return err
				}
				txMetadata := tx.GetMetadata(transaction, txIndexInsideBlock, block.GetIndex(), block.GetTimestamp())
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

	this.target = currentTarget

	return nil
}

func (b *Blockchain) ProcessNewBlocks(blocks []safebox.SerializedBlock, preSave *func(safebox.SafeboxBase) error) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.safebox.Rollback()
	defer b.txPoolApplyAndInvalidateUnsafe()

	if err := b.processNewBlocksUnsafe(blocks, preSave); err != nil {
		return err
	}

	b.safebox.Merge()
	_, safeboxHash, _ := b.safebox.GetState()
	copy(b.prevSafeboxHash, safeboxHash)

	return nil
}

func (b *Blockchain) ProcessNewBlock(block safebox.SerializedBlock, broadcast bool) error {
	err := b.ProcessNewBlocks([]safebox.SerializedBlock{block}, nil)
	if err == nil && broadcast {
		b.BlocksUpdates <- block
	}
	return err
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
		mainBlock, err := this.GetBlock(header.Index)
		if err != nil {
			return err
		}
		if !bytes.Equal(mainBlock.GetPrevSafeBoxHash(), header.PrevSafeboxHash) {
			return fmt.Errorf("alternate chain parent not found")
		}
	}

	snapshot, err := this.LoadNearestSnapshot(blocks[0].Header.Index)
	if err != nil {
		return fmt.Errorf("failed to find nearest snapshot: %v", err)
	}

	snapshotHeight := snapshot.GetHeight()
	mainBlock, err := this.GetBlock(snapshotHeight - 1)
	if err != nil {
		return err
	}

	newBlockchain := newBlockchain(this.newSafeboxCallback, this.storage, snapshot, mainBlock.GetTarget())
	currentTarget := newBlockchain.target
	for index := snapshotHeight; index < blocks[0].Header.Index; index++ {
		block, err := this.GetBlock(index)
		if err != nil {
			return err
		}
		currentTarget, _, err = newBlockchain.addBlock(currentTarget, block)
		if err != nil {
			return err
		}
	}
	newBlockchain.target = currentTarget
	newBlockchain.safebox.Merge()

	this.lock.Lock()
	defer this.lock.Unlock()

	cumulativeDifficultyCheck := func(altSafebox safebox.SafeboxBase) error {
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

func (b *Blockchain) TxPoolAddOperation(transaction tx.CommonOperation, broadcast bool) (new bool, err error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.txPoolAddOperationUnsafe(transaction, broadcast)
}

func (b *Blockchain) txPoolAddOperationUnsafe(transaction tx.CommonOperation, broadcast bool) (new bool, err error) {
	_, exists := b.txPool.Get(tx.GetTxIdString(transaction))
	if exists {
		return false, nil
	}

	if _, err := b.safebox.ProcessOperations(nil, 0, []tx.CommonOperation{transaction}, nil); err != nil {
		return false, err
	}

	b.txPool.Add(tx.GetTxIdString(transaction), transaction)
	if broadcast {
		b.TxPoolUpdates <- transaction
	}
	return !exists, nil
}

func (b *Blockchain) txPoolApplyAndInvalidateUnsafe() {
	oldTxPool := b.txPool
	b.txPool = iterator.New()
	for item := range oldTxPool.Iter() {
		transaction := item.Value.(tx.CommonOperation)
		b.txPoolAddOperationUnsafe(transaction, true)
	}
}

func (b *Blockchain) GetBlockTemplate(miner *crypto.Public, payload []byte, time *uint32, nonce uint32) (block safebox.BlockBase, template []byte, reservedOffset int, err error) {
	if block, err = b.getPendingBlock(miner, payload, time, nonce); err != nil {
		return nil, nil, 0, err
	}
	template, reservedOffset = safebox.GetBlockHashingBlob(block)
	return block, template, reservedOffset, nil
}

func (b *Blockchain) UnmarshalHashingBlob(blob []byte) (miner *crypto.Public, nonce uint32, timestamp uint32, payload []byte, err error) {
	return safebox.UnmarshalHashingBlob(blob)
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
		Operations: tx.ToTxSerialized(block.GetOperations()),
	}
}

func (b *Blockchain) GetTopBlock() (safebox.BlockBase, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if height := b.GetHeight(); height > 0 {
		return b.GetBlock(height - 1)
	}

	return b.getPendingBlockUnsafe(nil, nil, nil, 0)
}

func (b *Blockchain) GetTxPool() map[tx.CommonOperation]tx.TxMetadata {
	b.lock.RLock()
	defer b.lock.RUnlock()

	result := make(map[tx.CommonOperation]tx.TxMetadata)

	i := uint32(0)
	time := uint32(time.Now().Unix())
	for item := range b.txPool.Iter() {
		transaction := item.Value.(tx.CommonOperation)
		result[transaction] = tx.GetMetadata(transaction, i, 0, time)
		i++
	}

	return result
}

func (b *Blockchain) getPendingBlock(miner *crypto.Public, payload []byte, timestamp *uint32, nonce uint32) (safebox.BlockBase, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.getPendingBlockUnsafe(miner, payload, timestamp, nonce)
}

func (b *Blockchain) getPendingBlockUnsafe(miner *crypto.Public, payload []byte, timestamp *uint32, nonce uint32) (safebox.BlockBase, error) {
	var blockTimestamp uint32
	if timestamp == nil {
		blockTimestamp = uint32(time.Now().Unix())
	} else {
		blockTimestamp = *timestamp
	}

	if payload == nil {
		payload = []byte("")
	}

	var minerSerialized []byte
	if miner == nil {
		minerSerialized = utils.Serialize(crypto.NewKey(nil, 0, nil, big.NewInt(0), big.NewInt(0)).Public)
	} else {
		minerSerialized = utils.Serialize(miner)
	}

	txes := make([]tx.CommonOperation, 0)
	for item := range b.txPool.Iter() {
		transaction := item.Value.(tx.CommonOperation)
		txes = append(txes, transaction)
	}

	meta := safebox.BlockMetadata{
		Index: b.safebox.GetHeight(),
		Miner: minerSerialized,
		Version: common.Version{
			Major: 1,
			Minor: 1,
		},
		Timestamp:       blockTimestamp,
		Target:          b.target.GetCompact(),
		Nonce:           nonce,
		Payload:         payload,
		PrevSafeBoxHash: make([]byte, len(b.prevSafeboxHash)),
		Operations:      tx.ToTxSerialized(txes),
	}
	copy(meta.PrevSafeBoxHash[:], b.prevSafeboxHash[:])
	block, err := safebox.NewBlock(&meta)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (this *Blockchain) GetBlock(index uint32) (safebox.BlockBase, error) {
	var serialized []byte
	serialized, err := this.storage.GetBlock(index)
	if err != nil {
		return nil, err
	}
	var meta safebox.BlockMetadata
	if err = utils.Deserialize(&meta, bytes.NewBuffer(serialized)); err != nil {
		return nil, err
	}

	return safebox.NewBlock(&meta)
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

func (this *Blockchain) GetOperation(txRipemd160Hash [20]byte) (*tx.TxMetadata, tx.CommonOperation, error) {
	metadataSerialized, err := this.storage.GetTxMetadata(txRipemd160Hash)
	if err != nil {
		return nil, nil, err
	}

	return tx.TxFromMetadata(metadataSerialized)
}

func (b *Blockchain) AccountOperationsForEach(number uint32, offset uint32, limit uint32, fn func(operationId uint32, meta *tx.TxMetadata, tx tx.CommonOperation) bool) error {
	b.lock.RLock()
	defer b.lock.RUnlock()

	account := b.safebox.GetAccount(number)
	if account == nil {
		return fmt.Errorf("account doesn't exist")
	}

	limit = utils.MinUint32(limit, utils.MaxUint32(account.GetOperationsTotal(), offset)-offset)
	serializedTxes, err := b.storage.GetAccountTxesData(number, offset, limit)
	if err != nil {
		return err
	}

	for operationId, _ := range serializedTxes {
		if meta, tx, err := tx.TxFromMetadata(serializedTxes[operationId]); err == nil {
			if !fn(operationId, meta, tx) {
				return nil
			}
		} else {
			return err
		}
	}

	return nil
}

func (this *Blockchain) BlockOperationsForEach(index uint32, fn func(meta *tx.TxMetadata, tx tx.CommonOperation) bool) error {
	block, err := this.GetBlock(index)
	if err != nil {
		return fmt.Errorf("Failed to get block %d: %v", index, err)
	}

	operations := block.GetOperations()
	for index, _ := range operations {
		meta := tx.GetMetadata(operations[index], uint32(index), block.GetIndex(), block.GetTimestamp())
		if !fn(&meta, operations[index]) {
			return nil
		}
	}
	return nil
}

func (b *Blockchain) GetAccountsByPublicKey(public *crypto.Public) []uint32 {
	b.lock.RLock()
	defer b.lock.RUnlock()

	accounts := make([]uint32, 0)

	total := b.safebox.GetHeight() * defaults.AccountsPerBlock
	for current := uint32(0); current < total; current++ {
		account := b.safebox.GetAccount(current)
		if account == nil {
			break
		}
		if account.IsPublicKeyEqual(public) {
			accounts = append(accounts, current)
		}
	}

	return accounts
}

func (this *Blockchain) ExportSafebox() []byte {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.safebox.ToBlob()
}

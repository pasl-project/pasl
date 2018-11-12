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

package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/pasl-project/pasl/utils"
)

const (
	blocksCacheLimit = 509
)

const (
	tablePack      = "pack"
	tableBlock     = "block"
	tableTx        = "tx"
	tableAccountTx = "accountTx"
)

type Storage interface {
	Load(callback func(number uint32, serialized []byte) error) (height uint32, err error)
	LoadBlocks(toHeight *uint32, callback func(index uint32, serialized []byte) error) error
	Store(index uint32, data []byte, txes func(func(txRipemd160Hash [20]byte, txData []byte)), accountOperations func(func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte)), affectedPacks func(func(index uint32, data []byte))) error
	GetBlock(index uint32) (data []byte, err error)
	Flush() error
	GetTx(txRipemd160Hash [20]byte) (data []byte, err error)
	GetAccountTxesData(number uint32) (txData map[uint32][]byte, err error)
}

type StorageBoltDb struct {
	db               *bolt.DB
	accountsPerBlock uint32
	lock             sync.RWMutex
	blocksCache      map[uint32][]byte
	packsCache       map[uint32][]byte
	txesCache        map[[20]byte][]byte
	accountTxesCache map[*[8]byte]*[20]byte
}

func WithStorage(filename *string, accountsPerBlock uint32, fn func(storage Storage) error) error {
	_, err := os.Stat(*filename)
	firstRun := os.IsNotExist(err)
	db, err := bolt.Open(*filename, 0600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	storage := &StorageBoltDb{
		db:               db,
		accountsPerBlock: accountsPerBlock,
		blocksCache:      make(map[uint32][]byte),
		packsCache:       make(map[uint32][]byte),
		txesCache:        make(map[[20]byte][]byte),
		accountTxesCache: make(map[*[8]byte]*[20]byte),
	}

	if firstRun {
		if err = storage.createTables(); err != nil {
			return fmt.Errorf("Failed to initializ db %v", err)
		}
	}

	defer storage.Flush()
	return fn(storage)
}

func (this *StorageBoltDb) createTables() error {
	return this.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(tablePack)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tableAccountTx)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tableBlock)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tableTx)); err != nil {
			return err
		}
		return nil
	})
}

func (this *StorageBoltDb) Load(callback func(index uint32, serialized []byte) error) (height uint32, err error) {
	err = this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tableBlock)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableBlock)
		}
		height = uint32(bucket.Stats().KeyN)

		if bucket = tx.Bucket([]byte(tablePack)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tablePack)
		}
		cursor := bucket.Cursor()

		var pack uint32 = 0
		for key, value := cursor.First(); key != nil && pack < height; key, value = cursor.Next() {
			index := binary.BigEndian.Uint32(key)
			if err = callback(index, value); err != nil {
				return err
			}
			pack++
		}

		if pack != height {
			return fmt.Errorf("Failed to load account packs, loaded %d, height %d", pack, height)
		}

		return nil
	})
	return
}

func (this *StorageBoltDb) LoadBlocks(toHeight *uint32, callback func(index uint32, serialized []byte) error) error {
	return this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tableBlock)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableBlock)
		}

		var height uint32
		if toHeight == nil {
			height = uint32(bucket.Stats().KeyN)
		} else {
			height = *toHeight
		}

		cursor := bucket.Cursor()
		var total uint32 = 0
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			index := binary.BigEndian.Uint32(key)
			if index >= height {
				break
			}
			if index != total {
				return fmt.Errorf("Failed to load block, unexpected index %d, should be %d", index, total)
			}
			if err := callback(index, value); err != nil {
				return err
			}
			total++
		}

		if total != height {
			return fmt.Errorf("Failed to load blocks, loaded %d, height %d", total, height)
		}

		return nil
	})
}

func (this *StorageBoltDb) Store(index uint32, data []byte, txes func(func(txRipemd160Hash [20]byte, txData []byte)), accountOperations func(func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte)), affectedPacks func(func(number uint32, data []byte))) error {
	flush := false

	func() {
		this.lock.Lock()
		defer this.lock.Unlock()

		this.blocksCache[index] = make([]byte, len(data))
		copy(this.blocksCache[index], data)

		txes(func(txRipemd160Hash [20]byte, txData []byte) {
			this.txesCache[txRipemd160Hash] = make([]byte, len(txData))
			copy(this.txesCache[txRipemd160Hash], txData)
		})
		accountOperations(func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte) {
			numberAndId := [8]byte{}
			binary.BigEndian.PutUint32(numberAndId[0:4], number)
			binary.BigEndian.PutUint32(numberAndId[4:8], internalOperationId)
			this.accountTxesCache[&numberAndId] = &txRipemd160Hash
		})
		affectedPacks(func(index uint32, data []byte) {
			this.packsCache[index] = make([]byte, len(data))
			copy(this.packsCache[index], data)
		})

		flush = len(this.blocksCache) > blocksCacheLimit
	}()

	if flush {
		return this.Flush()
	}

	return nil
}

func (this *StorageBoltDb) Flush() error {
	err := this.db.Update(func(tx *bolt.Tx) (err error) {
		this.lock.Lock()
		defer this.lock.Unlock()

		err = (func() error {
			var bucket *bolt.Bucket
			if bucket = tx.Bucket([]byte(tableBlock)); bucket == nil {
				return fmt.Errorf("Table doesn't exist %s", tableBlock)
			}

			for index, data := range this.blocksCache {
				buffer := [4]byte{}
				binary.BigEndian.PutUint32(buffer[:], index)
				if err = bucket.Put(buffer[:], data); err != nil {
					return err
				}
			}

			if bucket = tx.Bucket([]byte(tablePack)); bucket == nil {
				return fmt.Errorf("Table doesn't exist %s", tablePack)
			}

			for number, data := range this.packsCache {
				buffer := [4]byte{}
				binary.BigEndian.PutUint32(buffer[:], number)
				if err = bucket.Put(buffer[:], data); err != nil {
					return err
				}
			}

			if bucket = tx.Bucket([]byte(tableTx)); bucket == nil {
				return fmt.Errorf("Table doesn't exist %s", tableTx)
			}

			for txRipemd160Hash, data := range this.txesCache {
				txRipemd160HashCopy := txRipemd160Hash
				if err = bucket.Put(txRipemd160HashCopy[:], data); err != nil {
					return err
				}
			}

			if bucket = tx.Bucket([]byte(tableAccountTx)); bucket == nil {
				return fmt.Errorf("Table doesn't exist %s", tableAccountTx)
			}

			for accountAndId, txRipemd160Hash := range this.accountTxesCache {
				if err = bucket.Put(accountAndId[:], txRipemd160Hash[:]); err != nil {
					return err
				}
			}

			return nil
		})()
		if err != nil {
			tx.Rollback()
			return err
		}

		return nil
	})

	if err == nil {
		this.blocksCache = make(map[uint32][]byte)
		this.packsCache = make(map[uint32][]byte)
		this.txesCache = make(map[[20]byte][]byte)
		this.accountTxesCache = make(map[*[8]byte]*[20]byte)
	}

	return err
}

func (this *StorageBoltDb) GetBlock(index uint32) (data []byte, err error) {
	this.lock.RLock()
	dataBuffer := this.blocksCache[index]
	this.lock.RUnlock()
	if dataBuffer != nil {
		data = make([]byte, len(dataBuffer))
		copy(data, dataBuffer)
		return data, nil
	}

	err = this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tableBlock)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableBlock)
		}
		var indexBuf [4]byte
		binary.BigEndian.PutUint32(indexBuf[:], index)
		if dataBuffer := bucket.Get(indexBuf[:]); dataBuffer != nil {
			data = make([]byte, len(dataBuffer))
			copy(data, dataBuffer)
		} else {
			return fmt.Errorf("Failed to get block #%d", index)
		}
		return nil
	})
	return
}

func (this *StorageBoltDb) GetTx(txRipemd160Hash [20]byte) (data []byte, err error) {
	this.lock.RLock()
	dataBuffer := this.txesCache[txRipemd160Hash]
	this.lock.RUnlock()
	if dataBuffer != nil {
		data = make([]byte, len(dataBuffer))
		copy(data, dataBuffer)
		return data, nil
	}

	err = this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tableTx)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableTx)
		}
		if dataBuffer := bucket.Get(txRipemd160Hash[:]); dataBuffer != nil {
			data = make([]byte, len(dataBuffer))
			copy(data, dataBuffer)
		} else {
			return fmt.Errorf("Failed to get tx %s", hex.EncodeToString(txRipemd160Hash[:]))
		}
		return nil
	})
	return
}

func (this *StorageBoltDb) GetAccountTxesData(number uint32) (txData map[uint32][]byte, err error) {
	txHashes := make(map[uint32]*[20]byte)
	this.lock.RLock()
	var current uint32
	var operationId uint32
	for numberAndId, txRipemd160Hash := range this.accountTxesCache {
		current = binary.BigEndian.Uint32(numberAndId[0:4])
		operationId = binary.BigEndian.Uint32(numberAndId[4:8])
		if current == number {
			txHashes[operationId] = txRipemd160Hash
		}
	}
	txData = make(map[uint32][]byte)
	for operationId, txRipemd160Hash := range txHashes {
		if dataBuffer, ok := this.txesCache[*txRipemd160Hash]; ok {
			txData[operationId] = make([]byte, len(dataBuffer))
			copy(txData[operationId][:], dataBuffer)
			delete(txHashes, operationId)
		}
	}
	this.lock.RUnlock()

	err = this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tableAccountTx)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableAccountTx)
		}

		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], number)

		c := bucket.Cursor()
		for k, v := c.Seek(buf[:]); k != nil; k, v = c.Next() {
			numberAndId := bytes.NewBuffer(k)
			if err := binary.Read(numberAndId, binary.BigEndian, &current); err != nil {
				utils.Panicf("Failed to unpack account number: %v", err)
			}
			if err := binary.Read(numberAndId, binary.BigEndian, &operationId); err != nil {
				utils.Panicf("Failed to unpack account number: %v", err)
			}
			if current != number {
				break
			}
			txRipemd160Hash := [20]byte{}
			copy(txRipemd160Hash[:], v)
			txHashes[operationId] = &txRipemd160Hash
		}

		if bucket = tx.Bucket([]byte(tableTx)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableTx)
		}

		for operationId, txRipemd160Hash := range txHashes {
			if dataBuffer := bucket.Get(txRipemd160Hash[:]); dataBuffer != nil {
				txData[operationId] = make([]byte, len(dataBuffer))
				copy(txData[operationId][:], dataBuffer)
			} else {
				return fmt.Errorf("Failed to get tx %s", hex.EncodeToString(txRipemd160Hash[:]))
			}
		}

		return nil
	})

	if err != nil {
		utils.Panicf("Failed to fetch txes from DB %v", err)
	}
	return txData, err
}

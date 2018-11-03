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
	blocksCacheLimit      = 50
	accountsCacheLimit    = 1000
	txesCacheLimit        = 100
	accountTxesCacheLimit = 100
)

const (
	tableAccount   = "account"
	tableBlock     = "block"
	tableTx        = "tx"
	tableAccountTx = "accountTx"
)

type Storage struct {
	db               *bolt.DB
	accountsPerBlock uint32
	lock             sync.RWMutex
	blocksCache      map[uint32][]byte
	accountsCache    map[uint32][]byte
	txesCache        map[[20]byte][]byte
	accountTxesCache map[*[8]byte]*[20]byte
}

func WithStorage(filename *string, accountsPerBlock uint32, fn func(storage *Storage) error) error {
	_, err := os.Stat(*filename)
	firstRun := os.IsNotExist(err)
	db, err := bolt.Open(*filename, 0600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	storage := &Storage{
		db:               db,
		accountsPerBlock: accountsPerBlock,
		blocksCache:      make(map[uint32][]byte),
		accountsCache:    make(map[uint32][]byte),
		txesCache:        make(map[[20]byte][]byte),
		accountTxesCache: make(map[*[8]byte]*[20]byte),
	}

	if firstRun {
		if err = storage.createTables(); err != nil {
			return fmt.Errorf("Failed to initializ db %v", err)
		}
	}

	defer storage.flush()
	return fn(storage)
}

func (this *Storage) createTables() error {
	return this.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(tableAccount)); err != nil {
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

func (this *Storage) Load(callback func(number uint32, serialized []byte) error) (height uint32, err error) {
	err = this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tableBlock)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableBlock)
		}
		height = uint32(bucket.Stats().KeyN)

		if bucket = tx.Bucket([]byte(tableAccount)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableAccount)
		}
		cursor := bucket.Cursor()

		var account uint32 = 0
		totalAccounts := height * this.accountsPerBlock
		for key, value := cursor.First(); key != nil && account < totalAccounts; key, value = cursor.Next() {
			number := binary.BigEndian.Uint32(key)
			if err = callback(number, value); err != nil {
				return err
			}
			account++
		}

		if account < totalAccounts {
			return fmt.Errorf("Failed to load accounts #%d - #%d", account, totalAccounts)
		}

		return nil
	})
	return
}

func (this *Storage) Store(index uint32, data []byte, txes func(func(txRipemd160Hash [20]byte, txData []byte)), accountOperations func(func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte)), affectedAccounts func(func(number uint32, data []byte) error) error) error {
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
		affectedAccounts(func(number uint32, data []byte) (err error) {
			this.accountsCache[number] = make([]byte, len(data))
			copy(this.accountsCache[number], data)
			return
		})

		flush = len(this.blocksCache) > blocksCacheLimit
		flush = flush || len(this.accountsCache) > accountsCacheLimit
		flush = flush || len(this.txesCache) > txesCacheLimit
		flush = flush || len(this.accountTxesCache) > accountTxesCacheLimit
	}()

	if flush {
		return this.flush()
	}

	return nil
}

func (this *Storage) flush() error {
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

			if bucket = tx.Bucket([]byte(tableAccount)); bucket == nil {
				return fmt.Errorf("Table doesn't exist %s", tableAccount)
			}

			for number, data := range this.accountsCache {
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
		this.accountsCache = make(map[uint32][]byte)
		this.txesCache = make(map[[20]byte][]byte)
		this.accountTxesCache = make(map[*[8]byte]*[20]byte)
	}

	return err
}

func (this *Storage) GetBlock(index uint32) (data []byte, err error) {
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

func (this *Storage) GetTx(txRipemd160Hash [20]byte) (data []byte, err error) {
	this.lock.RLock()
	dataBuffer := this.txesCache[txRipemd160Hash]
	this.lock.RUnlock()
	if data != nil {
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

func (this *Storage) GetAccountTxesData(number uint32) (txData map[uint32][]byte, err error) {
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

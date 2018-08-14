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
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/pasl-project/pasl/utils"
)

const (
	blocksCacheLimit   = 50
	accountsCacheLimit = 1000
)

type Storage struct {
	db               *bolt.DB
	accountsPerBlock uint32
	lock             sync.RWMutex
	blocksCache      map[uint32][]byte
	accountsCache    map[uint32][]byte
}

func WithStorage(accountsPerBlock uint32, fn func(storage *Storage) error) error {
	dataDir, err := utils.CreateDataDir()
	db, err := bolt.Open(filepath.Join(dataDir, "storage.db"), 0600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	storage := &Storage{
		db:               db,
		accountsPerBlock: accountsPerBlock,
		blocksCache:      make(map[uint32][]byte),
		accountsCache:    make(map[uint32][]byte),
	}

	defer storage.flush()
	return fn(storage)
}

func (this *Storage) Load(callback func(number uint32, serialized []byte) error) (height uint32, err error) {
	err = this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte("blocks")); bucket == nil {
			return nil
		}
		height = uint32(bucket.Stats().KeyN)

		if bucket = tx.Bucket([]byte("accounts")); bucket == nil {
			return nil
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

func (this *Storage) Store(index uint32, data []byte, affectedAccounts func(func(number uint32, data []byte) error) error) error {
	flush := false

	func() {
		this.lock.Lock()
		defer this.lock.Unlock()

		this.blocksCache[index] = make([]byte, len(data))
		copy(this.blocksCache[index], data)

		affectedAccounts(func(number uint32, data []byte) (err error) {
			this.accountsCache[number] = make([]byte, len(data))
			copy(this.accountsCache[number], data)
			return
		})

		flush = len(this.blocksCache) > blocksCacheLimit || len(this.accountsCache) > accountsCacheLimit
	}()

	if flush {
		return this.flush()
	}

	return nil
}

func (this *Storage) flush() error {
	return this.db.Update(func(tx *bolt.Tx) (err error) {
		this.lock.Lock()
		defer this.lock.Unlock()

		err = (func() error {
			var bucket *bolt.Bucket
			if bucket, err = tx.CreateBucketIfNotExists([]byte("blocks")); err != nil {
				return err
			}

			var buffer [4]byte
			for index, data := range this.blocksCache {
				binary.BigEndian.PutUint32(buffer[:], index)
				if err = bucket.Put(buffer[:], data); err != nil {
					return err
				}
			}

			if bucket, err = tx.CreateBucketIfNotExists([]byte("accounts")); err != nil {
				return err
			}

			for number, data := range this.accountsCache {
				binary.BigEndian.PutUint32(buffer[:], number)
				if err = bucket.Put(buffer[:], data); err != nil {
					return err
				}
			}

			return nil
		})()
		if err != nil {
			tx.Rollback()
			return err
		}

		this.blocksCache = make(map[uint32][]byte)
		this.accountsCache = make(map[uint32][]byte)

		return nil
	})
}

func (this *Storage) GetBlock(index uint32) (data []byte, err error) {
	this.lock.RLock()
	data = this.blocksCache[index]
	this.lock.RUnlock()
	if data != nil {
		return data, nil
	}

	err = this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte("blocks")); bucket == nil {
			return nil
		}
		var indexBuf [4]byte
		binary.BigEndian.PutUint32(indexBuf[:], index)
		if data = bucket.Get(indexBuf[:]); data == nil {
			return fmt.Errorf("Failed to get block #%d", index)
		}
		return nil
	})
	return
}

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
	"errors"
	"fmt"
	"os"

	"github.com/boltdb/bolt"
	"github.com/pasl-project/pasl/utils"
)

const (
	blocksCacheLimit = 509
)

const (
	tableAccountTx  = "accountTx"
	tableBlock      = "block"
	tablePack       = "pack"
	tablePeers      = "peers"
	tableTx         = "tx"
	tableTxMetadata = "txMetadata"
)

var (
	ErrSafeboxInconsistent = errors.New("Failed to load safebox")
)

type StorageWritable interface {
	StoreBlock(context interface{}, index uint32, data []byte) error
	StoreTxHash(context interface{}, txRipemd160Hash [20]byte, blockIndex uint32, txIndexInsideBlock uint32) (uint64, error)
	StoreTxMetadata(context interface{}, txId uint64, txMetadata []byte) error
	StoreAccountOperation(context interface{}, number uint32, internalOperationId uint32, txId uint64) error
	StoreAccountPack(context interface{}, index uint32, data []byte) error
	StorePeers(context interface{}, peers func(func(address []byte, data []byte))) error
}

type Storage interface {
	Load(callback func(number uint32, serialized []byte) error) (height uint32, err error)
	LoadBlocks(toHeight *uint32, callback func(index uint32, serialized []byte) error) error
	WithWritable(fn func(storageWritable StorageWritable, context interface{}) error) error
	LoadPeers(peers func(address []byte, data []byte)) error
	GetBlock(index uint32) (data []byte, err error)
	GetTxMetadata(txRipemd160Hash [20]byte) (data []byte, err error)
	GetAccountTxesData(number uint32) (txData map[uint32][]byte, err error)
}

type StorageBoltDb struct {
	db *bolt.DB

	Storage
	StorageWritable
}

func WithStorage(filename *string, fn func(storage Storage) error) error {
	_, err := os.Stat(*filename)
	firstRun := os.IsNotExist(err)
	db, err := bolt.Open(*filename, 0600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	storage := &StorageBoltDb{
		db: db,
	}

	if firstRun {
		if err = storage.createTables(); err != nil {
			return fmt.Errorf("Failed to initializ db %v", err)
		}
	}

	return fn(storage)
}

func (this *StorageBoltDb) createTables() error {
	return this.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(tableAccountTx)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tableBlock)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tablePack)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tableTx)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tableTxMetadata)); err != nil {
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
		packs := uint32(bucket.Stats().KeyN)
		if packs != height {
			utils.Tracef("Packs loaded %d, height %d", packs, height)
			if packs < height {
				height = packs
			} else {
				return ErrSafeboxInconsistent
			}
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
			utils.Tracef("Loaded %d block", index)
			total++
		}

		if total != height {
			return fmt.Errorf("Failed to load blocks, loaded %d, height %d", total, height)
		}

		return nil
	})
}

func (this *StorageBoltDb) WithWritable(fn func(storageWritable StorageWritable, context interface{}) error) error {
	return this.db.Update(func(tx *bolt.Tx) error {
		return fn(this, tx)
	})
}

func (this *StorageBoltDb) StoreBlock(context interface{}, index uint32, data []byte) error {
	tx := context.(*bolt.Tx)

	var bucket *bolt.Bucket
	tableName := tableBlock
	if bucket = tx.Bucket([]byte(tableName)); bucket == nil {
		return fmt.Errorf("Table doesn't exist %s", tableName)
	}

	buffer := [4]byte{}
	binary.BigEndian.PutUint32(buffer[:], index)
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return bucket.Put(buffer[:], dataCopy)
}

func (this *StorageBoltDb) StoreTxHash(context interface{}, txRipemd160Hash [20]byte, blockIndex uint32, txIndexInsideBlock uint32) (uint64, error) {
	tx := context.(*bolt.Tx)

	var bucket *bolt.Bucket
	tableName := tableTx
	if bucket = tx.Bucket([]byte(tableName)); bucket == nil {
		return 0, fmt.Errorf("Table doesn't exist %s", tableName)
	}

	txID := uint64(blockIndex)*4294967296 + uint64(txIndexInsideBlock)
	buffer := bytes.NewBuffer([]byte(""))
	binary.Write(buffer, binary.BigEndian, txID)
	txRipemd160HashCopy := txRipemd160Hash
	if err := bucket.Put(txRipemd160HashCopy[:], buffer.Bytes()); err != nil {
		return 0, err
	}
	return txID, nil
}

func (this *StorageBoltDb) StoreTxMetadata(context interface{}, txID uint64, txMetadata []byte) error {
	tx := context.(*bolt.Tx)

	var bucket *bolt.Bucket
	tableName := tableTxMetadata
	if bucket = tx.Bucket([]byte(tableName)); bucket == nil {
		return fmt.Errorf("Table doesn't exist %s", tableName)
	}

	buffer := bytes.NewBuffer([]byte(""))
	binary.Write(buffer, binary.BigEndian, txID)
	txMetadataCopy := make([]byte, len(txMetadata))
	copy(txMetadataCopy, txMetadata)
	return bucket.Put(buffer.Bytes(), txMetadataCopy)
}

func (this *StorageBoltDb) StoreAccountOperation(context interface{}, number uint32, internalOperationId uint32, txId uint64) error {
	tx := context.(*bolt.Tx)

	var bucket *bolt.Bucket
	tableName := tableAccountTx
	if bucket = tx.Bucket([]byte(tableName)); bucket == nil {
		return fmt.Errorf("Table doesn't exist %s", tableName)
	}

	numberAndId := [8]byte{}
	binary.BigEndian.PutUint32(numberAndId[0:4], number)
	binary.BigEndian.PutUint32(numberAndId[4:8], internalOperationId)
	txId = uint64(bucket.Stats().KeyN)
	buffer := [8]byte{}
	binary.BigEndian.PutUint64(buffer[:], txId)
	return bucket.Put(numberAndId[:], buffer[:])
}

func (this *StorageBoltDb) StoreAccountPack(context interface{}, index uint32, data []byte) error {
	tx := context.(*bolt.Tx)

	var bucket *bolt.Bucket
	tableName := tablePack
	if bucket = tx.Bucket([]byte(tableName)); bucket == nil {
		return fmt.Errorf("Table doesn't exist %s", tableName)
	}

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	buffer := [4]byte{}
	binary.BigEndian.PutUint32(buffer[:], index)
	return bucket.Put(buffer[:], data)
}

func (this *StorageBoltDb) StorePeers(context interface{}, peers func(func(address []byte, data []byte))) (err error) {
	tx := context.(*bolt.Tx)

	var bucket *bolt.Bucket
	tableName := tablePeers
	if bucket = tx.Bucket([]byte(tableName)); bucket == nil {
		return fmt.Errorf("Table doesn't exist %s", tableName)
	}

	peers(func(address []byte, data []byte) {
		if err != nil {
			return
		}
		err = bucket.Put(address, data)
	})

	return err
}

func (this *StorageBoltDb) LoadPeers(peers func(address []byte, data []byte)) error {
	return this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tablePeers)); bucket == nil {
			return nil
		}

		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			peers(k, v)
		}
		return nil
	})
}

func (this *StorageBoltDb) GetBlock(index uint32) (data []byte, err error) {
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

func (this *StorageBoltDb) getTxId(tx *bolt.Tx, txRipemd160Hash [20]byte) (uint64, error) {
	var bucket *bolt.Bucket

	tableName := tableTx
	if bucket = tx.Bucket([]byte(tableName)); bucket == nil {
		return 0, fmt.Errorf("Table doesn't exist %s", tableName)
	}

	if txIdBuffer := bucket.Get(txRipemd160Hash[:]); txIdBuffer != nil {
		return binary.BigEndian.Uint64(txIdBuffer), nil
	}
	return 0, fmt.Errorf("Failed to get tx id by hash %x", txRipemd160Hash)
}

func (this *StorageBoltDb) getTxMetadata(boltTx *bolt.Tx, txId uint64) (metadata []byte, err error) {
	var bucket *bolt.Bucket
	tableName := tableTxMetadata
	if bucket = boltTx.Bucket([]byte(tableName)); bucket == nil {
		return nil, fmt.Errorf("Table doesn't exist %s", tableName)
	}

	txIdBuffer := [8]byte{}
	binary.BigEndian.PutUint64(txIdBuffer[:], txId)

	if metadataBuffer := bucket.Get(txIdBuffer[:]); metadataBuffer != nil {
		metadata = make([]byte, len(metadataBuffer))
		copy(metadata, metadataBuffer)
		return metadata, nil
	}

	return nil, fmt.Errorf("Failed to get tx metadata by id %s", txId)
}

func (this *StorageBoltDb) GetTxMetadata(txRipemd160Hash [20]byte) (metadata []byte, err error) {
	err = this.db.View(func(boltTx *bolt.Tx) error {
		txId, err := this.getTxId(boltTx, txRipemd160Hash)
		if err != nil {
			return err
		}

		if metadata, err = this.getTxMetadata(boltTx, txId); err != nil {
			return err
		}

		return nil
	})
	return
}

func (this *StorageBoltDb) GetAccountTxesData(number uint32) (txData map[uint32][]byte, err error) {
	return txData, this.db.View(func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket

		if bucket = tx.Bucket([]byte(tableAccountTx)); bucket == nil {
			return fmt.Errorf("Table doesn't exist %s", tableAccountTx)
		}

		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], number)

		txIds := make(map[uint32]uint64)
		var current uint32
		var operationId uint32
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
			txIds[operationId] = binary.BigEndian.Uint64(v)
		}

		for operationId, txId := range txIds {
			if metadata, err := this.getTxMetadata(tx, txId); err == nil {
				txData[operationId] = metadata
			} else {
				return err
			}
		}

		return nil
	})
}

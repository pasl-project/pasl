package storage

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"
)

func WriteREadTest(t *testing.T, flush bool) {
	dbFileName := "test.db"

	var txRipemd160Hash [20]byte
	txHash, _ := hex.DecodeString("1234567890123456789012345678901234567890")
	copy(txRipemd160Hash[:], txHash[:])
	serializedAccount, _ := hex.DecodeString("3456789012345678901234567890123456789012")
	serializedBlock, _ := hex.DecodeString("4567890123456789012345678901234567890123")
	serializedTx, _ := hex.DecodeString("2345678901234567890123456789012345678901")
	number := uint32(0x19001233)
	operationIndex := uint32(0x01293818)

	WithStorage(&dbFileName, 5, func(storage *Storage) error {
		storage.Store(0, serializedBlock,
			func(txes func(txRipemd160Hash [20]byte, txData []byte)) {
				txes(txRipemd160Hash, serializedTx)
			},
			func(accountOperations func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte)) {
				accountOperations(number, operationIndex, txRipemd160Hash)
			},
			func(fn func(number uint32, data []byte) error) error {
				return fn(number, serializedAccount)
			})
		if flush {
			storage.flush()
		}
		txes, err := storage.GetAccountTxes(number)
		if err != nil {
			t.FailNow()
		}
		if txHash, ok := txes[operationIndex]; !ok {
			t.FailNow()
		} else if bytes.Compare(txHash[:], txRipemd160Hash[:]) != 0 {
			t.FailNow()
		}
		return nil
	})
	os.Remove(dbFileName)
}

func TestWriteFlushRead(t *testing.T) {
	WriteREadTest(t, true)
}

func TestWriteRead(t *testing.T) {
	WriteREadTest(t, true)
}

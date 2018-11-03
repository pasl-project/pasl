package storage

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"
)

func WriteReadTest(t *testing.T, flush bool) {
	dbFileName := "test.db"

	var txRipemd160Hash [20]byte
	txHash, _ := hex.DecodeString("1234567890123456789012345678901234567890")
	copy(txRipemd160Hash[:], txHash[:])
	var txRipemd160Hash2 [20]byte
	txHash2, _ := hex.DecodeString("7890123456789012345678901234567890123456")
	copy(txRipemd160Hash2[:], txHash2[:])
	var txRipemd160Hash3 [20]byte
	txHash3, _ := hex.DecodeString("8901234567890123456789012345678901234567")
	copy(txRipemd160Hash3[:], txHash3[:])
	serializedAccount, _ := hex.DecodeString("3456789012345678901234567890123456789012")
	serializedAccount2, _ := hex.DecodeString("6789012345678901234567890123456789012345")
	serializedBlock, _ := hex.DecodeString("4567890123456789012345678901234567890123")
	serializedTx, _ := hex.DecodeString("2345678901234567890123456789012345678901")
	serializedTx2, _ := hex.DecodeString("5678901234567890123456789012345678901234")
	number := uint32(0x19001233)
	number2 := uint32(0x19001244)
	operationIndex := uint32(0x01293818)
	operationIndex2 := uint32(0x01293814)

	defer os.Remove(dbFileName)
	if WithStorage(&dbFileName, 5, func(storage Storage) error {
		storage.Store(0, serializedBlock,
			func(txes func(txRipemd160Hash [20]byte, txData []byte)) {
				txes(txRipemd160Hash, serializedTx)
				txes(txRipemd160Hash2, serializedTx2)
			},
			func(accountOperations func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte)) {
				accountOperations(number, operationIndex, txRipemd160Hash)
				accountOperations(number2, operationIndex2, txRipemd160Hash2)
			},
			func(fn func(number uint32, data []byte) error) error {
				if err := fn(number, serializedAccount); err != nil {
					t.Fatal()
				}
				if err := fn(number2, serializedAccount2); err != nil {
					t.Fatal()
				}
				return nil
			})

		if flush {
			storage.Flush()
		}

		{
			txesData, err := storage.GetAccountTxesData(number)
			if err != nil {
				t.Fatal()
			}
			if txData, ok := txesData[operationIndex]; !ok {
				t.Fatal()
			} else if bytes.Compare(txData[:], serializedTx[:]) != 0 {
				t.Fatal()
			}
		}

		{
			if txData, err := storage.GetTx(txRipemd160Hash); txData == nil || err != nil {
				t.Fatal()
			}
			if txData, err := storage.GetTx(txRipemd160Hash3); txData != nil || err == nil {
				t.Fatal()
			}
		}

		return nil
	}) != nil {
		t.Fatal()
	}
}

func TestWriteFlushRead(t *testing.T) {
	WriteReadTest(t, true)
}

func TestWriteRead(t *testing.T) {
	WriteReadTest(t, false)
}

func TestFailOpen(t *testing.T) {
	dbFileName := ":"
	if err := WithStorage(&dbFileName, 5, func(storage Storage) error {
		return nil
	}); err == nil {
		t.Fatal()
	}
}

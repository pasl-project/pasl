package storage

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"os"
	"testing"
)

func TestWriteRead(t *testing.T) {
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
	// serializedAccount2, _ := hex.DecodeString("6789012345678901234567890123456789012345")
	serializedBlock, _ := hex.DecodeString("4567890123456789012345678901234567890123")
	serializedTx, _ := hex.DecodeString("2345678901234567890123456789012345678901")
	// serializedTx2, _ := hex.DecodeString("5678901234567890123456789012345678901234")
	number := uint32(0x19001233)
	// number2 := uint32(0x19001244)
	operationIndex := uint32(0x01293818)
	// operationIndex2 := uint32(0x01293814)

	snapshotIndex := rand.Uint32()
	snapshotData := make([]byte, 6)
	if _, err := rand.Read(snapshotData); err != nil {
		t.Fatal(err)
	}

	peers := make(map[string]string, 5)
	{
		buf := make([]byte, 6)
		for each := 0; each < 5; each++ {
			if _, err := rand.Read(buf); err != nil {
				t.Fatal(err)
			}
			address := hex.EncodeToString(buf)
			if _, err := rand.Read(buf); err != nil {
				t.Fatal(err)
			}
			data := hex.EncodeToString(buf)
			peers[address] = data
		}
	}

	defer os.Remove(dbFileName)
	if err := WithStorage(&dbFileName, func(s Storage) error {
		if err := s.WithWritable(func(s StorageWritable, ctx interface{}) error {
			if err := s.StoreBlock(ctx, 0, serializedBlock); err != nil {
				t.Fatal(err)
			}
			txID, err := s.StoreTxHash(ctx, txRipemd160Hash, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.StoreTxMetadata(ctx, txID, serializedTx); err != nil {
				t.Fatal(err)
			}
			if err := s.StoreAccountOperation(ctx, number, operationIndex, txID); err != nil {
				t.Fatal(err)
			}
			if err := s.StoreAccountPack(ctx, 0, serializedAccount); err != nil {
				t.Fatal(err)
			}

			if err := s.StorePeers(ctx, func(fn func(address []byte, data []byte)) {
				for each := range peers {
					fn([]byte(each), []byte(peers[each]))
				}
			}); err != nil {
				t.Fatal(err)
			}

			if err := s.StoreSnapshot(ctx, snapshotIndex, snapshotData); err != nil {
				t.Fatal(err)
			}

			return nil
		}); err != nil {
			t.Fatal(err)
		}

		{
			txesData, err := s.GetAccountTxesData(number)
			if err != nil {
				t.Fatal(err)
			}
			if txData, ok := txesData[operationIndex]; !ok {
				t.Fatal(err)
			} else if bytes.Compare(txData[:], serializedTx[:]) != 0 {
				t.Fatal(err)
			}
		}

		{
			if txData, err := s.GetTxMetadata(txRipemd160Hash); txData == nil || err != nil {
				t.Fatal(err)
			}
			if txData, err := s.GetTxMetadata(txRipemd160Hash3); txData != nil || err == nil {
				t.Fatal(err)
			}
		}

		{
			if err := s.LoadPeers(func(address []byte, data []byte) {
				if _, ok := peers[string(address)]; !ok {
					t.Fatalf("invalid peer address")
				}
				if !bytes.Equal([]byte(peers[string(address)]), data) {
					t.Fatalf("invalid peer data")
				}
				delete(peers, string(address))
			}); err != nil {
				t.Fatal(err)
			}
			if len(peers) != 0 {
				t.Fatalf("failed to load some peer")
			}
		}

		{
			snapshots := s.ListSnapshots()
			if snapshots[0] != snapshotIndex {
				t.Fatalf("invalid snapshot index")
			}
			if !bytes.Equal(s.LoadSnapshot(snapshots[0]), snapshotData) {
				t.Fatalf("invalid snapshot data")
			}
			if err := s.WithWritable(func(s StorageWritable, ctx interface{}) error {
				return s.DropSnapshot(ctx, snapshots[0])
			}); err != nil {
				t.Fatal(err)
			}
			if len(s.ListSnapshots()) != 0 {
				t.Fatalf("failed to drop snapshot")
			}
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFailOpen(t *testing.T) {
	dbFileName := ":"
	if err := WithStorage(&dbFileName, func(storage Storage) error {
		return nil
	}); err == nil {
		t.Fatal(err)
	}
}

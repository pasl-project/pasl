package blockchain

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"

	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/storage"
	"github.com/pasl-project/pasl/utils"
)

type EmptyStorage struct {
	storage.Storage
}

func (storage *EmptyStorage) Load(callback func(number uint32, serialized []byte) error) (height uint32, err error) {
	return 0, nil
}
func (storage *EmptyStorage) Store(index uint32, data []byte, txes func(func(txRipemd160Hash [20]byte, txData []byte)), accountOperations func(func(number uint32, internalOperationId uint32, txRipemd160Hash [20]byte)), affectedAccounts func(func(number uint32, data []byte))) error {
	return nil
}
func (storage *EmptyStorage) Flush() error {
	return nil
}
func (storage *EmptyStorage) GetBlock(index uint32) (data []byte, err error) {
	return nil, os.ErrNotExist
}
func (storage *EmptyStorage) GetTx(txRipemd160Hash [20]byte) (data []byte, err error) {
	return nil, os.ErrNotExist
}
func (storage *EmptyStorage) GetAccountTxesData(number uint32) (txData map[uint32][]byte, err error) {
	return nil, os.ErrNotExist
}

func TestPendingBlock(t *testing.T) {
	blockchain, err := NewBlockchain(&EmptyStorage{}, nil)
	if err != nil {
		t.Fatal()
	}

	height := blockchain.GetHeight()
	if blockchain.GetBlock(height) != nil {
		t.Fatal()
	}

	valid, _ := hex.DecodeString("030100010000000000060000000000000020a1070000000000000000000000000000000000000000240000000000002000dc9388917fb00065999f25bde135617677c7020a3aea916098b39ede89e37a222000e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85520000000000000000000000000000000000000000000000000000000000000000000")
	blockTimestamp := uint32(0)
	got := utils.Serialize(blockchain.SerializeBlockHeader(blockchain.GetPendingBlock(&blockTimestamp), false, true))
	if !bytes.Equal(valid, got) {
		t.Fatalf("\n%s !=\n%s", hex.EncodeToString(valid), hex.EncodeToString(got))
	}
}

func TestDeserializeAndPow(t *testing.T) {
	blockchain, err := NewBlockchain(&EmptyStorage{}, nil)
	if err != nil {
		t.Fatal()
	}

	height := blockchain.GetHeight()
	if height != 0 {
		t.Fatal()
	}
	if blockchain.GetBlock(height) != nil {
		t.Fatal()
	}

	rawBlock, _ := hex.DecodeString("0201000100000000004600ca02200059a6ef47d508cdd935d9841dc377555697b414c7a9daaa9ba289f9cee6fedd3220004ba82df4966794b2b33e1db8f8d7e18bc0d401012db9a169d22eaaa321cad41e20a107000000000000000000000000009f2f92580000002470a2f7322a004e6577204e6f646520322f312f323031372031313a35363a3333202d20204275696c643a742f312d2d2d2000dc9388917fb00065999f25bde135617677c7020a3aea916098b39ede89e37a222000e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b8552000000000000eae7a91b748c735a5338a11715d815101e0c075f7c60fa52b769ec700000000")
	powFromRaw := rawBlock[223:255]
	var blockSerialized safebox.SerializedBlock
	if err = utils.Deserialize(&blockSerialized, bytes.NewBuffer(rawBlock)); err != nil {
		t.Fatal()
	}

	if block, err := blockchain.AddBlockSerialized(&blockSerialized, nil); err != nil {
		t.Fatal()
	} else {
		height = blockchain.GetHeight()
		if height != 1 {
			t.Fatal()
		}

		pow := blockchain.GetBlockPow(block)
		if !bytes.Equal(pow, powFromRaw) {
			t.Fatalf("\n%s !=\n%s", hex.EncodeToString(powFromRaw), hex.EncodeToString(pow))
		}
	}

}

package blockchain

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/pasl-project/pasl/accounter"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/safebox/tx"

	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/storage"
	"github.com/pasl-project/pasl/utils"
)

type MemoryStorage struct {
	accountPacks map[uint32][]byte
	blocks       map[uint32][]byte
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		accountPacks: make(map[uint32][]byte),
		blocks:       make(map[uint32][]byte),
	}
}

func (storage *MemoryStorage) Load(callback func(number uint32, serialized []byte) error) (height uint32, err error) {
	return 0, nil
}
func (storage *MemoryStorage) LoadBlocks(toHeight *uint32, callback func(index uint32, serialized []byte) error) error {
	return fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) LoadPeers(peers func(address []byte, data []byte)) error {
	return fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) ListSnapshots() []uint32 {
	return nil
}
func (storage *MemoryStorage) LoadSnapshot(height uint32) (serialized []byte) {
	return nil
}
func (storage *MemoryStorage) GetBlock(index uint32) (data []byte, err error) {
	if data, ok := storage.blocks[index]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("block not found")
}
func (storage *MemoryStorage) GetTxMetadata(txRipemd160Hash [20]byte) (data []byte, err error) {
	return nil, fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) GetAccountTxesData(number uint32, offset uint32, limit uint32) (txData map[uint32][]byte, err error) {
	return nil, fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) WithWritable(fn func(storageWritable storage.StorageWritable, context interface{}) error) error {
	return fn(storage, interface{}(nil))
}

func (storage *MemoryStorage) StoreBlock(context interface{}, index uint32, data []byte) error {
	storage.blocks[index] = data
	return nil
}
func (storage *MemoryStorage) StoreTxHash(context interface{}, txRipemd160Hash [20]byte, blockIndex uint32, txIndexInsideBlock uint32) (uint64, error) {
	return 0, fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) StoreTxMetadata(context interface{}, txId uint64, txMetadata []byte) error {
	return fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) StoreAccountOperation(context interface{}, number uint32, internalOperationId uint32, txId uint64) error {
	return fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) StoreAccountPack(context interface{}, index uint32, data []byte) error {
	storage.accountPacks[index] = data
	return nil
}
func (storage *MemoryStorage) StorePeers(context interface{}, peers func(func(address []byte, data []byte))) error {
	return fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) StoreSnapshot(context interface{}, number uint32, serialized []byte) error {
	return fmt.Errorf("not implemented")
}
func (storage *MemoryStorage) DropSnapshot(context interface{}, height uint32) error {
	return fmt.Errorf("not implemented")
}

type MockSafebox struct {
	hash []byte
}

func (s *MockSafebox) ToBlob() []byte {
	return nil
}
func (s *MockSafebox) GetHashBuffer() []byte {
	return nil
}
func (s *MockSafebox) GetHeight() uint32 {
	return 0
}
func (s *MockSafebox) GetState() (height uint32, safeboxHash []byte, cumulativeDifficulty *big.Int) {
	return 0, s.hash, nil
}
func (s *MockSafebox) GetFork() safebox.Fork {
	return safebox.GetActiveFork(0, nil)
}
func (s *MockSafebox) GetForkByHeight(height uint32, prevSafeboxHash []byte) safebox.Fork {
	return safebox.GetActiveFork(height, nil)
}
func (s *MockSafebox) SetFork(fork safebox.Fork) {}
func (s *MockSafebox) Validate(operation tx.CommonOperation) error {
	return nil
}
func (s *MockSafebox) Merge()    {}
func (s *MockSafebox) Rollback() {}
func (s *MockSafebox) GetUpdatedPacks() []uint32 {
	return nil
}
func (s *MockSafebox) ProcessOperations(miner *crypto.Public, timestamp uint32, operations []tx.CommonOperation, difficulty *big.Int) (map[*accounter.Account]map[uint32]uint32, error) {
	rand.Read(s.hash)
	return nil, nil
}
func (s *MockSafebox) GetLastTimestamps(count uint32) (timestamps []uint32) {
	return nil
}
func (s *MockSafebox) GetHashrate(blockIndex, blocksCount uint32) uint64 {
	return 0
}
func (s *MockSafebox) GetAccount(number uint32) *accounter.Account {
	return nil
}
func (s *MockSafebox) GetAccountPackSerialized(index uint32) ([]byte, error) {
	return nil, nil
}
func (s *MockSafebox) SerializeAccounter() ([]byte, error) {
	return nil, nil
}

func TestPendingBlockSafebox(t *testing.T) {
	blockchain, _ := NewBlockchain(func(accounter *accounter.Accounter) safebox.SafeboxBase {
		return &MockSafebox{
			hash: make([]byte, sha256.Size),
		}
	}, NewMemoryStorage(), nil)

	_, safeboxHash, _ := blockchain.GetState()
	initialSafeboxHash := make([]byte, sha256.Size)
	copy(initialSafeboxHash, safeboxHash)

	public, _ := crypto.NewKeyByType(crypto.NIDsecp256k1)
	transaction := tx.Transfer{
		Source:      0,
		OperationId: 1,
		Destination: 2,
		Amount:      3,
		Fee:         4,
		Payload:     nil,
		PublicKey:   *public.Public,
	}
	if _, err := blockchain.TxPoolAddOperation(&transaction, false); err != nil {
		t.Fatal(err)
	}

	block, err := blockchain.getPendingBlock(nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(initialSafeboxHash, block.GetPrevSafeBoxHash()) {
		t.FailNow()
	}
}

func TestPendingBlock(t *testing.T) {
	blockchain, err := NewBlockchain(safebox.NewSafebox, NewMemoryStorage(), nil)
	if err != nil {
		t.Fatal()
	}

	height := blockchain.GetHeight()
	if _, err := blockchain.GetBlock(height); err == nil {
		t.Fatal()
	}

	valid, _ := hex.DecodeString("030100010000000000060000000000000020a1070000000000000000000000000000000000000000240000000000002000dc9388917fb00065999f25bde135617677c7020a3aea916098b39ede89e37a222000e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85520000000000000000000000000000000000000000000000000000000000000000000")
	blockTimestamp := uint32(0)
	block, err := blockchain.getPendingBlock(nil, nil, &blockTimestamp, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := utils.Serialize(blockchain.SerializeBlockHeader(block, false, true))
	if !bytes.Equal(valid, got) {
		t.Fatalf("\n%s !=\n%s", hex.EncodeToString(valid), hex.EncodeToString(got))
	}
}

func TestDeserializeAndPow(t *testing.T) {
	blockchain, err := NewBlockchain(safebox.NewSafebox, NewMemoryStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	height := blockchain.GetHeight()
	if height != 0 {
		t.Fatal()
	}
	if _, err := blockchain.GetBlock(height); err == nil {
		t.Fatal()
	}

	rawBlock, _ := hex.DecodeString("0201000100000000004600ca02200059a6ef47d508cdd935d9841dc377555697b414c7a9daaa9ba289f9cee6fedd3220004ba82df4966794b2b33e1db8f8d7e18bc0d401012db9a169d22eaaa321cad41e20a107000000000000000000000000009f2f92580000002470a2f7322a004e6577204e6f646520322f312f323031372031313a35363a3333202d20204275696c643a742f312d2d2d2000dc9388917fb00065999f25bde135617677c7020a3aea916098b39ede89e37a222000e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b8552000000000000eae7a91b748c735a5338a11715d815101e0c075f7c60fa52b769ec700000000")
	powFromRaw := rawBlock[223:255]
	var blockSerialized safebox.SerializedBlock
	if err = utils.Deserialize(&blockSerialized, bytes.NewBuffer(rawBlock)); err != nil {
		t.Fatal(err)
	}

	if err := blockchain.ProcessNewBlock(blockSerialized, false); err != nil {
		t.Fatal(err)
	} else {
		height = blockchain.GetHeight()
		if height != 1 {
			t.Fatal()
		}

		block, err := blockchain.GetBlock(blockSerialized.Header.Index)
		if err != nil {
			t.Fatal(err)
		}
		pow := blockchain.GetBlockPow(block)
		if !bytes.Equal(pow, powFromRaw) {
			t.Fatalf("\n%s !=\n%s", hex.EncodeToString(powFromRaw), hex.EncodeToString(pow))
		}
	}
}

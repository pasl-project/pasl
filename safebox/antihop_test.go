package safebox

import (
	"bytes"
	"testing"

	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/safebox/tx"
)

type mockBlock struct {
	miner   *crypto.Public
	nonce   uint32
	payload []byte
	target  common.TargetBase
	version common.Version
}

func (m *mockBlock) GetIndex() uint32 {
	return 0
}
func (m *mockBlock) GetMiner() *crypto.Public {
	return m.miner
}
func (m *mockBlock) GetReward() uint64 {
	return 0
}
func (m *mockBlock) GetFee() uint64 {
	return 0
}
func (m *mockBlock) GetHash() []byte {
	return nil
}
func (m *mockBlock) GetVersion() common.Version {
	return m.version
}
func (m *mockBlock) GetTimestamp() uint32 {
	return 0
}
func (m *mockBlock) GetTarget() common.TargetBase {
	return m.target
}
func (m *mockBlock) GetNonce() uint32 {
	return m.nonce
}
func (m *mockBlock) GetPayload() []byte {
	return m.payload
}
func (m *mockBlock) GetPrevSafeBoxHash() []byte {
	return nil
}
func (m *mockBlock) GetOperationsHash() []byte {
	return nil
}
func (m *mockBlock) GetOperations() []tx.CommonOperation {
	return nil
}

func TestBlockHashingBlob(t *testing.T) {
	antiHop := antiHopDiff{}

	miner, err := crypto.NewKeyByType(crypto.NIDsecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	var block BlockBase = &mockBlock{
		miner:   miner.Public,
		payload: []byte("test payload"),
		target:  common.NewTarget(0),
		version: common.Version{
			Major: 1,
			Minor: 2,
		},
	}
	template, _ := antiHop.GetBlockHashingBlob(block)

	public, nonce, timestamp, payload, err := UnmarshalHashingBlob(template)
	if err != nil {
		t.Fatal(err)
	}
	if !block.GetMiner().Equal(public) {
		t.FailNow()
	}
	if block.GetNonce() != nonce {
		t.FailNow()
	}
	if block.GetTimestamp() != timestamp {
		t.FailNow()
	}
	if !bytes.Equal(block.GetPayload(), payload) {
		t.FailNow()
	}
}

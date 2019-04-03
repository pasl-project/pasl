package safebox

import (
	"bytes"
	"crypto/sha256"
	"io"

	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/utils"
)

type part1 struct {
	Index   uint32
	Miner   crypto.PublicSerialized
	Reward  uint64
	Version common.Version
	Target  uint32
}
type part2 struct {
	PrevSafeboxHash [32]byte
	OperationsHash  [32]byte
	Fee             uint32
	Timestamp       uint32
	Nonce           uint32
}

const part2Size = 32 + 32 + 4 + 4 + 4

func GetBlockPow(hashingBlob []byte) []byte {
	hash := sha256.Sum256(hashingBlob)
	pow := sha256.Sum256(hash[:])
	return pow[:]
}

func GetBlockHashingBlob(block BlockBase) (template []byte, reservedOffset int) {
	toHash := utils.Serialize(part1{
		Index:   block.GetIndex(),
		Miner:   block.GetMiner().Serialized(),
		Reward:  block.GetReward(),
		Version: block.GetVersion(),
		Target:  block.GetTarget().GetCompact(),
	})

	reservedOffset = len(toHash)

	toHash = append(toHash, block.GetPayload()...)

	p2 := part2{
		Fee:       uint32(block.GetFee()),
		Timestamp: block.GetTimestamp(),
		Nonce:     block.GetNonce(),
	}
	copy(p2.PrevSafeboxHash[:], block.GetPrevSafeBoxHash())
	copy(p2.OperationsHash[:], block.GetOperationsHash())

	toHash = append(toHash, utils.Serialize(p2)...)

	return toHash, reservedOffset
}

func UnmarshalHashingBlob(blob []byte) (miner *crypto.Public, nonce uint32, timestamp uint32, payload []byte, err error) {
	r := bytes.NewBuffer(blob)

	p1 := part1{}
	if err := utils.Deserialize(&p1, r); err != nil {
		return nil, 0, 0, nil, err
	}

	payload = make([]byte, r.Len()-part2Size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, 0, 0, nil, err
	}

	p2 := part2{}
	if err := utils.Deserialize(&p2, r); err != nil {
		return nil, 0, 0, nil, err
	}

	miner = &crypto.Public{}
	if err := crypto.PublicFromSerialized(miner, p1.Miner.TypeId, p1.Miner.X, p1.Miner.Y); err != nil {
		return nil, 0, 0, nil, err
	}

	return miner, p2.Nonce, p2.Timestamp, payload, nil
}

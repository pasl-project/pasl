package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"

	"github.com/pasl-project/pasl/safebox/tx"

	"github.com/pasl-project/pasl/safebox"

	"github.com/pasl-project/pasl/blockchain"
	"github.com/pasl-project/pasl/utils"

	"github.com/pasl-project/pasl/network"
)

type Api struct {
	network.API

	blockchain *blockchain.Blockchain
}

func NewApi(blockchain *blockchain.Blockchain) *Api {
	return &Api{
		blockchain: blockchain,
	}
}

func (this *Api) GetBlockCount(ctx context.Context) (int, error) {
	height, _, _ := this.blockchain.GetState()
	return int(height), nil
}

func (this *Api) GetBlock(ctx context.Context, params *struct{ Block uint32 }) (*network.Block, error) {
	blockMeta := this.blockchain.GetBlock(params.Block)
	if blockMeta == nil {
		return nil, errors.New("Not found")
	}
	operations := blockMeta.GetOperations()
	fee := uint64(0)
	for _, tx := range operations {
		fee = fee + tx.GetFee()
	}
	height, _, _ := this.blockchain.GetState()
	return &network.Block{
		Block:       blockMeta.GetIndex(),
		EncPubkey:   hex.EncodeToString(utils.Serialize(blockMeta.GetMiner().Serialized())),
		Fee:         float64(fee) / 10000,
		Hashratekhs: this.blockchain.GetHashrate(blockMeta.GetIndex(), 50) / 1000,
		Maturation:  utils.MaxUint32(height, blockMeta.GetIndex()+1) - blockMeta.GetIndex() - 1,
		Nonce:       blockMeta.GetNonce(),
		Operations:  uint64(len(operations)),
		Oph:         hex.EncodeToString(blockMeta.GetOperationsHash()),
		Payload:     hex.EncodeToString(blockMeta.GetPayload()),
		Pow:         hex.EncodeToString(this.blockchain.GetBlockPow(blockMeta)),
		Reward:      float64(blockMeta.GetReward()) / 10000,
		Sbh:         hex.EncodeToString(blockMeta.GetPrevSafeBoxHash()),
		Target:      blockMeta.GetTarget().GetCompact(),
		Timestamp:   blockMeta.GetTimestamp(),
		Ver:         blockMeta.GetVersion().Major,
		VerA:        blockMeta.GetVersion().Minor,
	}, nil
}

func (this *Api) GetAccount(ctx context.Context, params *struct{ Account uint32 }) (*network.Account, error) {
	account := this.blockchain.GetAccount(params.Account)
	if account == nil {
		return nil, errors.New("Not found")
	}
	return &network.Account{
		Account:    account.GetNumber(),
		Balance:    float64(account.GetBalance()) / 10000,
		EncPubkey:  hex.EncodeToString(utils.Serialize(account.GetPublicKeySerialized())),
		NOperation: account.GetOperationsCount(),
		UpdatedB:   account.GetUpdatedIndex(),
	}, nil
}

func txToNetwork(meta *tx.TxMetadata, tx *tx.Tx) network.Operation {
	return network.Operation{
		Account:        tx.GetAccount(),
		Amount:         float64(tx.GetAmount()) / -10000,
		Block:          meta.BlockIndex,
		Dest_account:   tx.GetDestAccount(),
		Fee:            float64(tx.GetFee()) / 10000,
		Opblock:        meta.Index, // TODO: consider to drop the field
		Ophash:         tx.GetTxIdString(),
		Optxt:          nil,
		Optype:         uint8(tx.Type),
		Payload:        hex.EncodeToString(tx.GetPayload()),
		Sender_account: tx.GetAccount(),
		Time:           meta.Time,
	}
}

func (this *Api) GetPending(ctx context.Context) ([]network.Operation, error) {
	response := make([]network.Operation, 0)
	this.blockchain.TxPoolForEach(func(meta *tx.TxMetadata, tx *tx.Tx) bool {
		response = append(response, txToNetwork(meta, tx))
		return true
	})
	return response, nil
}

func (this *Api) ExecuteOperations(ctx context.Context, params *struct{ RawOperations string }) (bool, error) {
	rawOperations, err := hex.DecodeString(params.RawOperations)
	if err != nil {
		utils.Tracef("Error: %v", err)
		return false, errors.New("Failed to decode hex inout")
	}

	operationsSet := safebox.SerializedOperations{}
	if err := utils.Deserialize(&operationsSet, bytes.NewBuffer(rawOperations)); err != nil {
		utils.Tracef("Error: %v", err)
		return false, errors.New("Failed to deserialize operations set")
	}

	any := false
	for _, tx := range operationsSet.Operations {
		_, err := this.blockchain.TxPoolAddOperation(&tx)
		if err != nil {
			utils.Tracef("Error: %v", err)
		} else if !any {
			any = true
		}
	}

	return true, nil
}

func (this *Api) FindOperation(ctx context.Context, params *struct{ Ophash string }) (*network.Operation, error) {
	ophash, err := hex.DecodeString(params.Ophash)
	if err != nil {
		return nil, errors.New("Failed to decode ophash")
	}
	var txRipemd160Hash [20]byte
	copy(txRipemd160Hash[:], ophash[12:])

	if meta, tx, err := this.blockchain.GetOperation(txRipemd160Hash); err == nil {
		operation := txToNetwork(meta, tx)
		return &operation, nil
	} else {
		return nil, errors.New("Not found")
	}
}

func (this *Api) GetAccountOperations(ctx context.Context, params *struct{ Account uint32 }) ([]network.Operation, error) {
	result := make([]network.Operation, 0)
	if err := this.blockchain.AccountOperationsForEach(params.Account, func(operationId uint32, meta *tx.TxMetadata, tx *tx.Tx) bool {
		result = append(result, txToNetwork(meta, tx))
		return true
	}); err != nil {
		return nil, errors.New("Not found")
	}
	return result, nil
}

func (this *Api) GetBlockOperations(ctx context.Context, params *struct{ Block uint32 }) ([]network.Operation, error) {
	result := make([]network.Operation, 0)
	if err := this.blockchain.BlockOperationsForEach(params.Block, func(meta *tx.TxMetadata, tx *tx.Tx) bool {
		result = append(result, txToNetwork(meta, tx))
		return true
	}); err != nil {
		return nil, errors.New("Not found")
	}
	return result, nil
}

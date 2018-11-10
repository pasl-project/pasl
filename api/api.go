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
		Fee:         fee,
		Hashratekhs: this.blockchain.GetHashrate(blockMeta.GetIndex(), 50) / 1000,
		Maturation:  utils.MaxUint32(height, blockMeta.GetIndex()+1) - blockMeta.GetIndex() - 1,
		Nonce:       blockMeta.GetNonce(),
		Operations:  uint64(len(operations)),
		Oph:         hex.EncodeToString(blockMeta.GetOperationsHash()),
		Payload:     hex.EncodeToString(blockMeta.GetPayload()),
		Pow:         hex.EncodeToString(this.blockchain.GetBlockPow(blockMeta)),
		Reward:      blockMeta.GetReward(),
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
		Account:    account.Number,
		Balance:    account.Balance,
		EncPubkey:  hex.EncodeToString(utils.Serialize(account.PublicKey.Serialized())),
		NOperation: account.Operations,
		UpdatedB:   account.UpdatedIndex,
	}, nil
}

func txToNetwork(tx *tx.Tx) network.Operation {
	return network.Operation{
		Account:        tx.GetAccount(),
		Amount:         tx.GetAmount(),
		Block:          0,
		Dest_account:   tx.GetDestAccount(),
		Fee:            tx.GetFee(),
		Opblock:        0, // TODO: consider to drop the field
		Ophash:         tx.GetTxIdString(),
		Optxt:          "", // TODO: consider to drop the field
		Optype:         uint8(tx.Type),
		Payload:        hex.EncodeToString(tx.GetPayload()),
		Sender_account: tx.GetAccount(),
		Time:           0,
	}
}

func (this *Api) GetPending(ctx context.Context) ([]network.Operation, error) {
	txes := this.blockchain.GetTxPool()
	response := make([]network.Operation, 0)
	for _, tx := range txes {
		response = append(response, txToNetwork(&tx))
	}
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
		_, err := this.blockchain.AddOperation(&tx)
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
	tx := this.blockchain.GetOperation(txRipemd160Hash)
	if tx == nil {
		return nil, errors.New("Not found")
	}
	operation := txToNetwork(tx)
	return &operation, nil
}

func (this *Api) GetAccountOperations(ctx context.Context, params *struct{ Account uint32 }) ([]network.Operation, error) {
	txes := this.blockchain.GetAccountOperations(params.Account)
	if txes == nil {
		return nil, errors.New("Not found")
	}

	result := make([]network.Operation, 0)
	for _, tx := range txes {
		result = append(result, txToNetwork(tx))
	}
	return result, nil
}

func (this *Api) GetBlockOperations(ctx context.Context, params *struct{ Block uint32 }) ([]network.Operation, error) {
	txes := this.blockchain.GetBlockOperations(params.Block)
	if txes == nil {
		return nil, errors.New("Not found")
	}

	result := make([]network.Operation, 0)
	for index, _ := range txes {
		result = append(result, txToNetwork(&txes[index]))
	}
	return result, nil
}

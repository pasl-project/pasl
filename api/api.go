package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/pasl-project/pasl/blockchain"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/network"
	"github.com/pasl-project/pasl/safebox"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/utils"
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

func (a *Api) GetHandlers() map[string]interface{} {
	return map[string]interface{}{
		"getblockcount":        a.GetBlockCount,
		"getblock":             a.GetBlock,
		"getaccount":           a.GetAccount,
		"getpendings":          a.GetPending,
		"executeoperations":    a.ExecuteOperations,
		"findoperation":        a.FindOperation,
		"getaccountoperations": a.GetAccountOperations,
		"getblockoperations":   a.GetBlockOperations,
		"getpubkeyaccounts":    a.GetPubKeyAccounts,
		"getblocktemplate":     a.GetBlockTemplate,
		"submitblock":          a.SubmitBlock,
	}
}

func (this *Api) GetBlockCount(context.Context, *struct{}) (int, error) {
	height := this.blockchain.GetHeight()
	return int(height), nil
}

func (this *Api) GetBlock(_ context.Context, params *struct{ Block uint32 }) (*network.Block, error) {
	blockMeta, err := this.blockchain.GetBlock(params.Block)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	operations := blockMeta.GetOperations()
	fee := uint64(0)
	for _, tx := range operations {
		fee = fee + tx.GetFee()
	}
	height := this.blockchain.GetHeight()
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

func (this *Api) GetAccount(_ context.Context, params *struct{ Account uint32 }) (*network.Account, error) {
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

func txToNetwork(meta *tx.TxMetadata, transaction tx.CommonOperation) network.Operation {
	return network.Operation{
		Account:        transaction.GetAccount(),
		Amount:         float64(transaction.GetAmount()) / 10000,
		Block:          meta.BlockIndex,
		Dest_account:   transaction.GetDestAccount(),
		Fee:            float64(transaction.GetFee()) / 10000,
		Opblock:        meta.Index, // TODO: consider to drop the field
		Ophash:         tx.GetTxIdString(transaction),
		Optxt:          nil,
		Optype:         uint8(transaction.GetType()),
		Payload:        hex.EncodeToString(transaction.GetPayload()),
		Sender_account: transaction.GetAccount(),
		Time:           meta.Time,
	}
}

func (this *Api) GetPending(context.Context, *struct{}) ([]network.Operation, error) {
	response := make([]network.Operation, 0)

	for tx, meta := range this.blockchain.GetTxPool() {
		response = append(response, txToNetwork(&meta, tx))
	}

	return response, nil
}

func (this *Api) ExecuteOperations(_ context.Context, params *struct{ RawOperations string }) (bool, error) {
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
		_, err := this.blockchain.TxPoolAddOperation(tx, true)
		if err != nil {
			utils.Tracef("error: %v", err)
		} else if !any {
			any = true
		}
	}

	return any, nil
}

func (this *Api) FindOperation(_ context.Context, params *struct{ Ophash string }) (*network.Operation, error) {
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

func (this *Api) GetAccountOperations(_ context.Context, params *struct {
	Account uint32
	Offset  *uint32
	Limit   uint32
}) ([]network.Operation, error) {
	account := this.blockchain.GetAccount(params.Account)
	if account == nil {
		return nil, errors.New("Not found")
	}

	limit := utils.MaxUint32(params.Limit, 100)
	var offset uint32
	if params.Offset != nil {
		offset = utils.MinUint32(account.GetOperationsTotal(), *params.Offset)
	} else {
		offset = utils.MaxUint32(account.GetOperationsTotal(), limit) - limit
	}
	result := make([]network.Operation, 0, limit)
	if err := this.blockchain.AccountOperationsForEach(params.Account, offset, limit, func(operationId uint32, meta *tx.TxMetadata, tx tx.CommonOperation) bool {
		result = append(result, txToNetwork(meta, tx))
		return true
	}); err != nil {
		return nil, errors.New("Not found")
	}
	return result, nil
}

func (this *Api) GetBlockOperations(_ context.Context, params *struct{ Block uint32 }) ([]network.Operation, error) {
	result := make([]network.Operation, 0)
	if err := this.blockchain.BlockOperationsForEach(params.Block, func(meta *tx.TxMetadata, tx tx.CommonOperation) bool {
		result = append(result, txToNetwork(meta, tx))
		return true
	}); err != nil {
		return nil, errors.New("Not found")
	}
	return result, nil
}

func (this *Api) GetPubKeyAccounts(_ context.Context, params *struct{ B58_pubkey string }) ([]uint32, error) {
	public, err := crypto.PublicFromBase58(params.B58_pubkey)
	if err != nil {
		return nil, err
	}
	return this.blockchain.GetAccountsByPublicKey(public), nil
}

func (a *Api) GetBlockTemplate(_ context.Context, params *struct {
	Reserve_size   uint64
	Wallet_address string
}) (*network.BlockTemplate, error) {
	miner, err := crypto.PublicFromBase58(params.Wallet_address)
	if err != nil {
		return nil, err
	}

	block, template, reservedOffset, err := a.blockchain.GetBlockTemplate(miner, make([]byte, params.Reserve_size), nil, 0)
	if err != nil {
		return nil, err
	}
	templateHex := hex.EncodeToString(template)
	return &network.BlockTemplate{
		Difficulty:         block.GetTarget().GetDifficulty().Uint64(),
		Height:             uint64(block.GetIndex()),
		Expected_reward:    block.GetReward(),
		Reserved_offset:    uint64(reservedOffset),
		Prev_hash:          hex.EncodeToString(block.GetPrevSafeBoxHash()),
		Blocktemplate_blob: templateHex,
		Blockhashing_blob:  templateHex,
		Status:             "OK",
	}, nil
}

func (a *Api) SubmitBlock(_ context.Context, params []string) (*network.SubmitBlock, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("expecting single argument")
	}

	template, err := hex.DecodeString(params[0])
	if err != nil {
		return nil, err
	}

	miner, nonce, timestamp, payload, err := a.blockchain.UnmarshalHashingBlob(template)
	if err != nil {
		return nil, err
	}

	block, _, _, err := a.blockchain.GetBlockTemplate(miner, payload, &timestamp, nonce)
	if err != nil {
		return nil, err
	}
	if err := a.blockchain.ProcessNewBlock(a.blockchain.SerializeBlock(block), true); err != nil {
		return nil, err
	}

	return &network.SubmitBlock{
		Status: "OK",
	}, nil
}

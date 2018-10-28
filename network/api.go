package network

import (
	"context"
)

type Block struct {
	Block       uint32 `json:"block"`
	Enc_pubkey  string `json:"enc_pubkey"`
	Fee         uint64 `json:"fee"`
	Hashratekhs uint64 `json:"hashratekhs"`
	Maturation  uint32 `json:"maturation"`
	Nonce       uint32 `json:"nonce"`
	Operations  uint64 `json:"operations"`
	Oph         string `json:"oph"`
	Payload     string `json:"payload"`
	Pow         string `json:"pow"`
	Reward      uint64 `json:"reward"`
	Sbh         string `json:"sbh"`
	Target      uint32 `json:"target"`
	Timestamp   uint32 `json:"timestamp"`
	Ver         uint16 `json:"ver"`
	Ver_a       uint16 `json:"ver_a"`
}

type Account struct {
	Account     uint32 `json:"account"`
	Balance     uint64 `json:"balance"`
	Enc_pubkey  string `json:"enc_pubkey"`
	N_operation uint32 `json:"n_operation"`
	Updated_b   uint32 `json:"updated_b"`
}

type Operation struct {
	Account        uint32 `json:"account"`
	Amount         uint64 `json:"amount"`
	Block          uint32 `json:"block"`
	Dest_account   uint32 `json:"dest_account"`
	Fee            uint64 `json:"fee"`
	Opblock        uint32 `json:"opblock"`
	Ophash         string `json:"ophash"`
	Optxt          string `json:"optxt"`
	Optype         uint8  `json:"optype"`
	Payload        string `json:"payload"`
	Sender_account uint32 `json:"sender_account"`
	Time           uint32 `json:"time"`
}

type Api interface {
	GetBlockCount(ctx context.Context) (int, error)
	GetBlock(ctx context.Context, params *struct{ Block uint32 }) (*Block, error)
	GetAccount(ctx context.Context, params *struct{ Account uint32 }) (*Account, error)
	GetPending(ctx context.Context) ([]Operation, error)
	ExecuteOperations(ctx context.Context, params *struct{ RawOperations string }) (bool, error)
}

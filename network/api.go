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

type Api interface {
	GetBlockCount(ctx context.Context) (int, error)
	GetBlock(ctx context.Context, params *struct{ Block uint32 }) (*Block, error)
	GetAccount(ctx context.Context, params *struct{ Account uint32 }) (*Account, error)
}

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

type Api interface {
	GetBlockCount(ctx context.Context) (int, error)
	GetBlock(ctx context.Context, params *struct{ Block uint32 }) (*Block, error)
}

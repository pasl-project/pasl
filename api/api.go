package api

import (
	"context"

	"github.com/pasl-project/pasl/blockchain"

	"github.com/pasl-project/pasl/network"
)

type Api struct {
	network.Api

	blockchain *blockchain.Blockchain
}

func NewApi(blockchain *blockchain.Blockchain) *Api {
	return &Api{
		blockchain: blockchain,
	}
}

func (this *Api) GetBlockCount(ctx context.Context) (int, error) {
	height, _ := this.blockchain.GetState()
	return int(height), nil
}

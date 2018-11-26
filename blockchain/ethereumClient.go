package blockchain

import (
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"github.com/singnet/snet-daemon/config"
)

type EthereumClient struct {
	EthClient *ethclient.Client
	RawClient *rpc.Client
}

var ethereumClient EthereumClient

func GetEthereumClient() (EthereumClient, error) {

	ethereumClient = *new(EthereumClient)
	if client, err := rpc.Dial(config.GetString(config.EthereumJsonRpcEndpointKey)); err != nil {
		return ethereumClient, errors.Wrap(err, "error creating RPC client")
	} else {
		ethereumClient.RawClient = client
		ethereumClient.EthClient = ethclient.NewClient(client)
	}
	return ethereumClient, nil

}

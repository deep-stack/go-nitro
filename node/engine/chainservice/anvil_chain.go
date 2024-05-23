package chainservice

import (
	"context"
	"math/big"
	"os/exec"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/statechannels/go-nitro/internal/chain"
	chainutils "github.com/statechannels/go-nitro/node/engine/chainservice/utils"
)

type AnvilChain struct {
	AnvilCmd  *exec.Cmd
	ChainOpts AnvilChainOpts
	ethClient *ethclient.Client
}

type AnvilChainOpts struct {
	ChainUrl        string
	ChainStartBlock uint64
	ChainAuthToken  string
	ChainPks        []string
	NaAddress       common.Address
	VpaAddress      common.Address
	CaAddress       common.Address
}

func NewAnvilChain() *AnvilChain {
	// TODO: Move internalchain startAnvil
	anvilCmd, _ := chain.StartAnvil()

	anvilChain := AnvilChain{}
	anvilChain.AnvilCmd = anvilCmd

	chainAuthToken := ""
	chainUrl := "ws://127.0.0.1:8545"
	chainPks := []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d", "5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"}

	ethClient, _, err := chainutils.ConnectToChain(
		context.Background(),
		chainUrl,
		chainAuthToken,
		common.Hex2Bytes(chainPks[0]),
	)
	if err != nil {
		panic(err)
	}
	anvilChain.ethClient = ethClient

	// TODO: Move internalchain deploycontract
	naAddress, vpaAddress, caAddress, _ := chain.DeployContracts(context.Background(), chainUrl, chainAuthToken, chainPks[0])

	anvilChain.ChainOpts = AnvilChainOpts{
		ChainUrl:        chainUrl,
		ChainStartBlock: 0,
		ChainAuthToken:  chainAuthToken,
		ChainPks:        chainPks,
		NaAddress:       naAddress,
		VpaAddress:      vpaAddress,
		CaAddress:       caAddress,
	}
	return &anvilChain
}

func (chain AnvilChain) GetAccountBalance(accountAddress common.Address) (*big.Int, error) {
	latestBlock, _ := chain.ethClient.BlockByNumber(context.Background(), nil)
	return chain.ethClient.BalanceAt(context.Background(), accountAddress, latestBlock.Header().Number)
}

func (chain AnvilChain) GetLatestBlock() (*types.Block, error) {
	return chain.ethClient.BlockByNumber(context.Background(), nil)
}

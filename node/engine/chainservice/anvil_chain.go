package chainservice

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	NitroAdjudicator "github.com/statechannels/go-nitro/node/engine/chainservice/adjudicator"
	ConsensusApp "github.com/statechannels/go-nitro/node/engine/chainservice/consensusapp"
	chainutils "github.com/statechannels/go-nitro/node/engine/chainservice/utils"
	VirtualPaymentApp "github.com/statechannels/go-nitro/node/engine/chainservice/virtualpaymentapp"
	"github.com/statechannels/go-nitro/types"
)

const (
	ChainAuthToken = ""
	ChainUrl       = "ws://127.0.0.1:8545"
)

var ChainPks = []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d", "5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"}

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

func NewAnvilChain() (*AnvilChain, error) {
	anvilChain := AnvilChain{}
	_, err := anvilChain.StartAnvil()
	if err != nil {
		return nil, err
	}

	ethClient, txSubmitter, err := chainutils.ConnectToChain(
		context.Background(),
		ChainUrl,
		ChainAuthToken,
		common.Hex2Bytes(ChainPks[0]),
	)
	if err != nil {
		panic(err)
	}
	anvilChain.ethClient = ethClient

	naAddress, vpaAddress, caAddress, _ := anvilChain.DeployContracts(context.Background(), ethClient, txSubmitter)

	anvilChain.ChainOpts = AnvilChainOpts{
		ChainUrl:        ChainUrl,
		ChainStartBlock: 0,
		ChainAuthToken:  ChainAuthToken,
		ChainPks:        ChainPks,
		NaAddress:       naAddress,
		VpaAddress:      vpaAddress,
		CaAddress:       caAddress,
	}
	return &anvilChain, nil
}

func (chain AnvilChain) GetAccountBalance(accountAddress common.Address) (*big.Int, error) {
	latestBlock, _ := chain.ethClient.BlockByNumber(context.Background(), nil)
	return chain.ethClient.BalanceAt(context.Background(), accountAddress, latestBlock.Header().Number)
}

func (chain AnvilChain) GetLatestBlock() (*ethTypes.Block, error) {
	return chain.ethClient.BlockByNumber(context.Background(), nil)
}

func (chain *AnvilChain) StartAnvil() (*exec.Cmd, error) {
	chain.AnvilCmd = exec.Command("anvil", "--chain-id", "1337", "--block-time", "1", "--silent")
	chain.AnvilCmd.Stdout = os.Stdout
	chain.AnvilCmd.Stderr = os.Stderr
	err := chain.AnvilCmd.Start()
	if err != nil {
		return &exec.Cmd{}, err
	}
	// If Anvil start successfully, delay by 1 second for the chain to initialize
	time.Sleep(1 * time.Second)
	return chain.AnvilCmd, nil
}

// DeployContracts deploys the NitroAdjudicator, VirtualPaymentApp and ConsensusApp contracts.
func (chain AnvilChain) DeployContracts(ctx context.Context, ethClient *ethclient.Client, txSubmitter *bind.TransactOpts) (na common.Address, vpa common.Address, ca common.Address, err error) {
	na, err = deployContract(ctx, "NitroAdjudicator", ethClient, txSubmitter, NitroAdjudicator.DeployNitroAdjudicator)
	if err != nil {
		return types.Address{}, types.Address{}, types.Address{}, err
	}

	vpa, err = deployContract(ctx, "VirtualPaymentApp", ethClient, txSubmitter, VirtualPaymentApp.DeployVirtualPaymentApp)
	if err != nil {
		return types.Address{}, types.Address{}, types.Address{}, err
	}

	ca, err = deployContract(ctx, "ConsensusApp", ethClient, txSubmitter, ConsensusApp.DeployConsensusApp)
	if err != nil {
		return types.Address{}, types.Address{}, types.Address{}, err
	}

	return
}

type contractBackend interface {
	NitroAdjudicator.NitroAdjudicator | VirtualPaymentApp.VirtualPaymentApp | ConsensusApp.ConsensusApp
}

// deployFunc is a function that deploys a contract and returns the contract address, backend, and transaction.
type deployFunc[T contractBackend] func(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *ethTypes.Transaction, *T, error)

// deployContract deploys a contract and waits for the transaction to be mined.
func deployContract[T contractBackend](ctx context.Context, name string, ethClient *ethclient.Client, txSubmitter *bind.TransactOpts, deploy deployFunc[T]) (types.Address, error) {
	a, tx, _, err := deploy(txSubmitter, ethClient)
	if err != nil {
		return types.Address{}, err
	}

	fmt.Printf("Waiting for %s deployment confirmation\n", name)
	_, err = bind.WaitMined(ctx, ethClient, tx)
	if err != nil {
		return types.Address{}, err
	}
	fmt.Printf("%s successfully deployed to %s\n", name, a.String())
	return a, nil
}

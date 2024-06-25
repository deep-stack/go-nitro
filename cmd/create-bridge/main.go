package main

import (
	"context"
	"os/exec"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/bridge"
	"github.com/statechannels/go-nitro/cmd/utils"
	"github.com/statechannels/go-nitro/internal/chain"
	"github.com/statechannels/go-nitro/internal/node"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	p2pms "github.com/statechannels/go-nitro/node/engine/messageservice/p2p-message-service"
	"github.com/statechannels/go-nitro/node/engine/store"
)

func run() ([]*exec.Cmd, error) {
	runningCmd := []*exec.Cmd{}

	const CHAIN_PK = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	const STATE_CHANNEL_PK = "0279651921cd800ac560c21ceea27aab0107b67daf436cdd25ce84cad30159b4"

	// start 2 anvil chains
	anvilCmdL1, err := chain.StartAnvil("8545")
	if err != nil {
		return runningCmd, err
	}
	runningCmd = append(runningCmd, anvilCmdL1)

	anvilCmdL2, err := chain.StartAnvil("8546")
	if err != nil {
		return runningCmd, err
	}
	runningCmd = append(runningCmd, anvilCmdL2)

	// Deploy contracts
	contractAddresses, err := chain.DeployContracts(context.Background(), "ws://127.0.0.1:8545", "", CHAIN_PK)
	if err != nil {
		return runningCmd, err
	}

	bridgeAddress, err := chain.DeployL2Contracts(context.Background(), "ws://127.0.0.1:8546", "", CHAIN_PK)
	if err != nil {
		return runningCmd, err
	}

	chainOptsL1 := chainservice.ChainOpts{
		ChainUrl:           "ws://127.0.0.1:8545",
		ChainStartBlockNum: 0,
		ChainAuthToken:     "",
		ChainPk:            CHAIN_PK,
		NaAddress:          contractAddresses.NaAddress,
		VpaAddress:         contractAddresses.VpaAddress,
		CaAddress:          contractAddresses.CaAddress,
	}

	chainOptsL2 := chainservice.L2ChainOpts{
		ChainUrl:           "ws://127.0.0.1:8546",
		ChainStartBlockNum: 0,
		ChainAuthToken:     "",
		ChainPk:            CHAIN_PK,
		BridgeAddress:      bridgeAddress,
	}

	storeOptsL1 := store.StoreOpts{
		PkBytes:            common.Hex2Bytes(STATE_CHANNEL_PK),
		UseDurableStore:    false,
		DurableStoreFolder: "",
	}

	storeOptsL2 := store.StoreOpts{
		PkBytes:            common.Hex2Bytes(STATE_CHANNEL_PK),
		UseDurableStore:    false,
		DurableStoreFolder: "",
	}

	messageOptsL1 := p2pms.MessageOpts{
		PkBytes:   common.Hex2Bytes(STATE_CHANNEL_PK),
		Port:      3005,
		BootPeers: nil,
		PublicIp:  "127.0.0.1",
	}

	// TODO: Discuss use of test message service between nodePrime and counterparty prime
	messageOptsL2 := p2pms.MessageOpts{
		PkBytes:   common.Hex2Bytes(STATE_CHANNEL_PK),
		Port:      3006,
		BootPeers: nil,
		PublicIp:  "127.0.0.1",
	}

	nodeL1, storeL1, _, _, err := node.InitializeNode(chainOptsL1, storeOptsL1, messageOptsL1)
	if err != nil {
		return runningCmd, err
	}
	nodeL2, storeL2, _, _, err := node.InitializeL2Node(chainOptsL2, storeOptsL2, messageOptsL2)
	if err != nil {
		return runningCmd, err
	}

	bridge := bridge.New(nodeL1, nodeL2, storeL1, storeL2)
	defer bridge.Close()

	utils.WaitForKillSignal()
	utils.StopCommands(runningCmd...)
	return nil, nil
}

func main() {
	runningCmd, err := run()
	if err != nil && runningCmd != nil {
		utils.StopCommands(runningCmd...)
	}
}

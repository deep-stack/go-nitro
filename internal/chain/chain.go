package chain

import (
	"context"
	"os/exec"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	chainutils "github.com/statechannels/go-nitro/node/engine/chainservice/utils"
	"github.com/statechannels/go-nitro/types"
)

func StartAnvil() (*exec.Cmd, error) {
	anvilChain := chainservice.AnvilChain{}
	return anvilChain.StartAnvil()
}

// DeployContracts deploys the NitroAdjudicator, VirtualPaymentApp and ConsensusApp contracts.
func DeployContracts(ctx context.Context, chainUrl, chainAuthToken, chainPk string) (na common.Address, vpa common.Address, ca common.Address, err error) {
	ethClient, txSubmitter, err := chainutils.ConnectToChain(context.Background(), chainUrl, chainAuthToken, common.Hex2Bytes(chainPk))
	if err != nil {
		return types.Address{}, types.Address{}, types.Address{}, err
	}

	anvilChain := chainservice.AnvilChain{}
	return anvilChain.DeployContracts(ctx, ethClient, txSubmitter)
}

import {HardhatRuntimeEnvironment} from 'hardhat/types';
import {DeployFunction} from 'hardhat-deploy/types';

const func: DeployFunction = async function (hre: HardhatRuntimeEnvironment) {
  const {
    deployments: {deploy},
    getNamedAccounts,
  } = hre;

  const {deployer} = await getNamedAccounts();

  await deploy('Token', {
    from: deployer,
    log: true,
    args: [deployer],
    // TODO: Set ownership when using deterministic deployment
    deterministicDeployment: process.env.DISABLE_DETERMINISTIC_DEPLOYMENT ? false : true,
  });
};
export default func;
func.tags = ['Token'];

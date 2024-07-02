import fs from 'fs';
import path from 'path';

import {HardhatRuntimeEnvironment} from 'hardhat/types';
import {DeployFunction} from 'hardhat-deploy/types';

const func: DeployFunction = async function (hre: HardhatRuntimeEnvironment) {
  const {deployments, getNamedAccounts, getChainId, network} = hre;
  const {deploy} = deployments;

  const {deployer} = await getNamedAccounts();

  const addressesFilePath = `hardhat-deployments/${network.name}/.contracts.env`;
  let contractAddresses = '';

  console.log('Working on chain id #', await getChainId());
  console.log('deployer', deployer);

  const deployResult = await deploy('Bridge', {
    from: deployer,
    log: true,
    // TODO: Set ownership when using deterministic deployment
    deterministicDeployment: process.env.DISABLE_DETERMINISTIC_DEPLOYMENT ? false : true,
  });

  contractAddresses = `${contractAddresses}export BRIDGE_ADDRESS=${deployResult.address}\n`;
  const outputFilePath = path.resolve(addressesFilePath);
  fs.writeFileSync(outputFilePath, contractAddresses);
  console.log('Contracts deployed, addresses written to', outputFilePath);
};
export default func;
func.tags = ['bridge'];

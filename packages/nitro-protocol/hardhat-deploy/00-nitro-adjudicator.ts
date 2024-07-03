import fs from 'fs';
import path from 'path';

import {HardhatRuntimeEnvironment} from 'hardhat/types';
import {DeployFunction} from 'hardhat-deploy/types';

const func: DeployFunction = async function (hre: HardhatRuntimeEnvironment) {
  const {deployments, getNamedAccounts, getChainId, network} = hre;
  const {deploy} = deployments;

  const {deployer} = await getNamedAccounts();

  const addressesFilePath = `hardhat-deployments/${network.name}/.contracts.env`;

  console.log('Working on chain id #', await getChainId());

  const naDeployResult = await deploy('NitroAdjudicator', {
    from: deployer,
    log: true,
    // TODO: Set ownership when using deterministic deployment
    deterministicDeployment: process.env.DISABLE_DETERMINISTIC_DEPLOYMENT ? false : true,
  });

  // TODO: Write to output file in post deploy script
  const contractAddress = `export NA_ADDRESS=${naDeployResult.address}\n`;
  const outputFilePath = path.resolve(addressesFilePath);
  fs.writeFileSync(outputFilePath, contractAddress, {flag: 'a'});
  console.log('Nitro adjudicator contract deployed, address written to', outputFilePath);
};
export default func;
func.tags = ['NitroAdjudicator', 'deploy'];

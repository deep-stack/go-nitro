import 'hardhat-deploy';
import 'hardhat-deploy-ethers';
import fs from 'fs';
import path from 'path';

import {HardhatRuntimeEnvironment} from 'hardhat/types';

module.exports = async (hre: HardhatRuntimeEnvironment) => {
  const {deployments, getNamedAccounts, getChainId, network} = hre;
  const {deploy} = deployments;
  const {deployer} = await getNamedAccounts();

  const addressesFilePath = `hardhat-deployments/${network.name}/.contracts.env`;
  let contractAddresses = '';

  console.log('Working on chain id #', await getChainId());
  console.log('deployer', deployer);

  try {
    const deployResult = await deploy('ConsensusApp', {
      from: deployer,
      log: true,
      // TODO: Set ownership when using deterministic deployment
      deterministicDeployment: process.env.DISABLE_DETERMINISTIC_DEPLOYMENT ? false : true,
    });
    contractAddresses = `${contractAddresses}export CA_ADDRESS=${deployResult.address}\n`;
  } catch (err) {
    const msg = err instanceof Error ? err.message : JSON.stringify(err);
    console.error(`Error when deploying contract: ${msg}`);
  }

  try {
    const deployResult = await deploy('VirtualPaymentApp', {
      from: deployer,
      log: true,
      // TODO: Set ownership when using deterministic deployment
      deterministicDeployment: process.env.DISABLE_DETERMINISTIC_DEPLOYMENT ? false : true,
    });
    contractAddresses = `${contractAddresses}export VPA_ADDRESS=${deployResult.address}\n`;
  } catch (err) {
    const msg = err instanceof Error ? err.message : JSON.stringify(err);
    console.error(`Error when deploying contract: ${msg}`);
  }

  const outputFilePath = path.resolve(addressesFilePath);
  fs.writeFileSync(outputFilePath, contractAddresses);
  console.log('Contracts deployed, addresses written to', outputFilePath);
};
module.exports.tags = ['deploy'];
module.exports.dependencies = ['NitroAdjudicator'];

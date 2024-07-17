import {ethers} from 'ethers';
import {abi as tokenABI} from './artifacts/contracts/Token.sol/Token.json'

// Connect to local Ethereum node
const provider = new ethers.providers.JsonRpcProvider('http://localhost:8545');

// Define addresses and token contract
const ownerAddress = '0xf1ac8Dd1f6D6F5c0dA99097c57ebF50CD99Ce293';
const spenderAddress = '0xb2592723B09F42937543f199A305ea6576dCb506';
const tokenAddress = '0x71d73E2F0908c003070b5d60F66Be8Bf4Ff0A54e';
// 0xb2592723B09F42937543f199A305ea6576dCb506
// Define the ABI of the ERC20 token contract

// Create the token contract object
const tokenContract = new ethers.Contract(tokenAddress, tokenABI, provider);

async function getAllowance(owner: string, spender: string): Promise<void> {
  try {
    const allowance: ethers.BigNumber = await tokenContract.balanceOf();
    console.log(`Allowance of spender ${spender} by owner ${owner} is: ${allowance.toString()}`);
  } catch (error) {
    console.error(`An error occurred: ${error}`);
  }
}

// Get the allowance
// getAllowance(ownerAddress, spenderAddress);

const filter = {
  fromBlock:400,
  toBlock:700
}

async function fetchLogs() {
    const logs = await provider.getLogs(filter);
    console.log('Logs:', logs);
}
// getAllowance(ownerAddress, spenderAddress);
fetchLogs()

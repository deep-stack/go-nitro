// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;
import {StatusManager} from './StatusManager.sol';
import {IBridge} from './interfaces/IBridge.sol';

contract Bridge is StatusManager, IBridge {

    // holdings[asset][channelId] is the amount of asset held against channel channelId. 0 address implies ETH
    mapping(address => mapping(bytes32 => uint256)) public holdings;

    // Owner only method to update mirrored ledger channel state
    function updateMirroredChannelStatus(
        bytes32 channelId,
        bytes32 stateHash,
        bytes memory outcomeBytes
    ) public virtual onlyOwner {
        _updateFingerprint(channelId, stateHash, keccak256(outcomeBytes));
        emit StatusUpdated(channelId, stateHash);
    }


    // Updates the holdings and mirrored ledger channel state
    function updateMirroredChannelStates(
        address asset,
        bytes32 channelId,
        uint256 amount,
        bytes32 stateHash,
        bytes memory outcomeBytes
    ) external onlyOwner {
        holdings[asset][channelId] = amount;
        updateMirroredChannelStatus(channelId, stateHash, outcomeBytes);
    }
}

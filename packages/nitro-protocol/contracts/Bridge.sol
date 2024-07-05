// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;

import {Ownable} from '@openzeppelin/contracts/access/Ownable.sol';
import {StatusManager} from './StatusManager.sol';
import {IBridge} from './interfaces/IBridge.sol';
import {MultiAssetHolder} from './MultiAssetHolder.sol';

contract Bridge is MultiAssetHolder, IBridge, Ownable {

    // Updates the holdings and mirrored ledger channel state
    function updateMirroredChannelStates(
        bytes32 channelId,
        bytes32 stateHash,
        bytes memory outcomeBytes,
        uint256 amount,
        address asset
    ) external onlyOwner {
        holdings[asset][channelId] = amount;

        _updateFingerprint(channelId, stateHash, keccak256(outcomeBytes));
        emit StatusUpdated(channelId, stateHash);
    }
}

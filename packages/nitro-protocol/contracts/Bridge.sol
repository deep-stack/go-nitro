// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;
import {StatusManager} from './StatusManager.sol';

contract Bridge is StatusManager {

  // Make `onlyOwner` modifier
  address private _owner;

  constructor() {
    _owner = msg.sender;
  }

  function owner() public view virtual returns (address) {
    return _owner;
  }

  modifier onlyOwner() {
    require(owner() == msg.sender, "Ownership Assertion: Caller of the function is not the owner.");
    _;
  }

  // Owner only method to save mirrored ledger channel state
  function saveMirroredChannelStatus( bytes32 channelId, bytes32 stateHash,bytes32 outcomeHash) public virtual onlyOwner {
    statusOf[channelId] = _generateStatus(
  ChannelData( 0, 0, stateHash, outcomeHash)
    );
  }

  // Owner only method to update mirrored ledger channel state
  function updateMirroredChannelStatus( bytes32 channelId, bytes32 stateHash,bytes32 outcomeHash) public virtual onlyOwner {
    _updateFingerprint(channelId, stateHash, outcomeHash);
  }

  function _updateFingerprint(
    bytes32 channelId,
    bytes32 stateHash,
    bytes32 outcomeHash
  ) internal {
    (uint48 turnNumRecord, uint48 finalizesAt, ) = _unpackStatus(channelId);

    bytes32 newStatus = _generateStatus(
        ChannelData(turnNumRecord, finalizesAt, stateHash, outcomeHash)
    );
    statusOf[channelId] = newStatus;
  }

  // Public getter function to get ledger channel state
  function getMirroredChannelStatus(bytes32 channelId) public view returns (bytes32) {
    return statusOf[channelId];
  }
}

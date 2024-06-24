// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;

interface IStatusManager {
    enum ChannelMode {
        Open,
        Challenge,
        Finalized
    }

    // Function to set map from l2ChannelId to l1ChannelId
    function setL2ToL1(bytes32 l1ChannelId, bytes32 l2ChannelId) external;

    // Function to retrieve the mapped value of l2ChannelId
    function getL2ToL1(bytes32 l2ChannelId) external view returns (bytes32);

    struct ChannelData {
        uint48 turnNumRecord;
        uint48 finalizesAt;
        bytes32 stateHash; // keccak256(abi.encode(State))
        bytes32 outcomeHash;
    }
}

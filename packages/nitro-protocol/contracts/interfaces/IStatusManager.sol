// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;

interface IStatusManager {
    enum ChannelMode {
        Open,
        Challenge,
        Finalized
    }

    // Function to generate map from l1ChannelId to l2ChannelId and vice-versa
    function generateMirror(bytes32 l1ChannelId, bytes32 l2ChannelId) external;

    // Function to retrieve the mapped value of l1ChannelId
    function getMirror(bytes32 l1ChannelId) external view returns (bytes32);

    // Function to retrieve the mapped value of l2ChannelId
    function getL1Channel(bytes32 l2ChannelId) external view returns (bytes32);

    struct ChannelData {
        uint48 turnNumRecord;
        uint48 finalizesAt;
        bytes32 stateHash; // keccak256(abi.encode(State))
        bytes32 outcomeHash;
    }
}

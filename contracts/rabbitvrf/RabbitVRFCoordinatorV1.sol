// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity 0.8.30;

/// @title RabbitVRFCoordinatorV1
/// @notice Consensus-facing EVM facade for Rabbit Chain VRF.
/// @dev Testnet V0.1. Public VRF activation remains disabled until the
///      complete Rabbit VRF activation gate is satisfied.
contract RabbitVRFCoordinatorV1 {
    /// @dev Canonical geth system-call sender.
    address public constant SYSTEM_ADDRESS =
        0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE;

    /// @dev Canonical Rabbit VRF Coordinator V1 address.
    address public constant COORDINATOR_ADDRESS =
        0xdFc21aeA108e3F527E5f236ebf354dc8262719da;

    uint256 public constant VERSION = 1;

    error OnlySystem();

    modifier onlySystem() {
        if (msg.sender != SYSTEM_ADDRESS) {
            revert OnlySystem();
        }
        _;
    }

    /// @notice Reserved system entry point for the canonical RabbitSwap TWAP
    ///         observation update.
    /// @dev The complete observation state transition is implemented in the
    ///      next protocol slice. The selector is introduced here so the
    ///      predeploy/install and system-call plumbing can be tested first.
    function systemObservePrice() external onlySystem {
    }
}

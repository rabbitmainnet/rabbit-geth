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

    /// @dev Canonical RabbitSwap tRUSD/tWRAB pair used for Testnet V0.1 pricing.
    address public constant RABBITSWAP_PAIR =
        address(bytes20(hex"8b9f4581b71964049ac6be03b22000132438b385"));

    uint256 public constant VERSION = 1;

    uint256 public constant VRF_BASE_FEE_TRUSD_BASE_UNITS = 10_000;
    uint256 public constant TWAP_MIN_WINDOW_SECONDS = 1_800;
    uint256 public constant TWAP_OBSERVATION_CADENCE_SECONDS = 300;
    uint256 public constant TWAP_MAX_AGE_SECONDS = 3_600;
    uint256 public constant TWAP_OBSERVATION_RING_SIZE = 16;

    bytes4 private constant GET_RESERVES_SELECTOR =
        bytes4(keccak256("getReserves()"));

    bytes4 private constant PRICE0_CUMULATIVE_LAST_SELECTOR =
        bytes4(keccak256("price0CumulativeLast()"));

    struct Observation {
        uint64 observedAt;
        uint256 price0Cumulative;
    }

    // Slot 0: count and newest ring index are packed together.
    uint8 private _observationCount;
    uint8 private _observationIndex;

    // The fixed Testnet V0.1 observation ring starts after the packed metadata.
    Observation[16] private _observations;

    error OnlySystem();
    error ObservationSlotOutOfRange();

    modifier onlySystem() {
        if (msg.sender != SYSTEM_ADDRESS) {
            revert OnlySystem();
        }
        _;
    }

    /// @notice Number of initialized observations currently retained.
    function observationCount() external view returns (uint8) {
        return _observationCount;
    }

    /// @notice Physical ring slot containing the newest observation.
    function observationIndex() external view returns (uint8) {
        return _observationIndex;
    }

    /// @notice Returns one physical observation-ring slot.
    function observationAt(
        uint8 slot
    ) external view returns (uint64 observedAt, uint256 price0Cumulative) {
        if (slot >= TWAP_OBSERVATION_RING_SIZE) {
            revert ObservationSlotOutOfRange();
        }

        Observation storage observation = _observations[slot];

        return (
            observation.observedAt,
            observation.price0Cumulative
        );
    }

    /// @notice Returns the newest initialized protocol observation.
    function latestObservation()
        external
        view
        returns (
            bool initialized,
            uint64 observedAt,
            uint256 price0Cumulative
        )
    {
        if (_observationCount == 0) {
            return (false, 0, 0);
        }

        Observation storage observation =
            _observations[_observationIndex];

        return (
            true,
            observation.observedAt,
            observation.price0Cumulative
        );
    }

    /// @notice Advances the canonical RabbitSwap TWAP observation ring.
    /// @dev Called by Rabbit consensus during PreExecution. Pair failure,
    ///      unavailable data or zero reserves deliberately produce no state
    ///      transition and never fabricate a fallback price.
    function systemObservePrice() external onlySystem {
        (
            bool valid,
            uint256 currentPrice0Cumulative
        ) = _currentPrice0Cumulative();

        if (!valid) {
            return;
        }

        if (block.timestamp > type(uint64).max) {
            return;
        }

        uint64 observedAt = uint64(block.timestamp);

        if (_observationCount == 0) {
            _observations[0] = Observation({
                observedAt: observedAt,
                price0Cumulative: currentPrice0Cumulative
            });

            _observationIndex = 0;
            _observationCount = 1;
            return;
        }

        Observation storage newest =
            _observations[_observationIndex];

        if (observedAt < newest.observedAt) {
            return;
        }

        if (
            uint256(observedAt - newest.observedAt)
                < TWAP_OBSERVATION_CADENCE_SECONDS
        ) {
            return;
        }

        uint8 nextIndex = uint8(
            (uint256(_observationIndex) + 1)
                % TWAP_OBSERVATION_RING_SIZE
        );

        _observations[nextIndex] = Observation({
            observedAt: observedAt,
            price0Cumulative: currentPrice0Cumulative
        });

        _observationIndex = nextIndex;

        if (_observationCount < TWAP_OBSERVATION_RING_SIZE) {
            _observationCount += 1;
        }
    }

    /// @dev Returns RabbitSwap price0 cumulative at the current block timestamp.
    ///      Arithmetic intentionally preserves RabbitSwap uint32 timestamp and
    ///      uint256 cumulative wraparound semantics.
    function _currentPrice0Cumulative()
        internal
        view
        returns (bool valid, uint256 cumulative)
    {
        (
            bool cumulativeCallOK,
            bytes memory cumulativeData
        ) = RABBITSWAP_PAIR.staticcall(
            abi.encodeWithSelector(
                PRICE0_CUMULATIVE_LAST_SELECTOR
            )
        );

        if (!cumulativeCallOK || cumulativeData.length != 32) {
            return (false, 0);
        }

        uint256 storedPrice0Cumulative;

        assembly {
            storedPrice0Cumulative := mload(
                add(cumulativeData, 0x20)
            )
        }

        (
            bool reservesCallOK,
            bytes memory reservesData
        ) = RABBITSWAP_PAIR.staticcall(
            abi.encodeWithSelector(GET_RESERVES_SELECTOR)
        );

        if (!reservesCallOK || reservesData.length != 96) {
            return (false, 0);
        }

        uint256 reserve0Word;
        uint256 reserve1Word;
        uint256 timestampWord;

        assembly {
            reserve0Word := mload(add(reservesData, 0x20))
            reserve1Word := mload(add(reservesData, 0x40))
            timestampWord := mload(add(reservesData, 0x60))
        }

        if (
            reserve0Word > type(uint112).max
                || reserve1Word > type(uint112).max
                || timestampWord > type(uint32).max
        ) {
            return (false, 0);
        }

        uint112 reserve0 = uint112(reserve0Word);
        uint112 reserve1 = uint112(reserve1Word);
        uint32 pairTimestampLast = uint32(timestampWord);

        if (reserve0 == 0 || reserve1 == 0) {
            return (false, 0);
        }

        uint32 currentTimestamp32 = uint32(block.timestamp);

        unchecked {
            uint32 elapsed32 =
                currentTimestamp32 - pairTimestampLast;

            if (elapsed32 == 0) {
                return (true, storedPrice0Cumulative);
            }

            uint256 price0X112 =
                (uint256(reserve1) << 112)
                / uint256(reserve0);

            cumulative =
                storedPrice0Cumulative
                + price0X112 * uint256(elapsed32);
        }

        return (true, cumulative);
    }
}

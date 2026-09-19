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

    uint256 private constant Q112 = 1 << 112;

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
    error PricingUnavailable();
    error FeeOverflow();

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

    /// @notice Returns the current canonical Rabbit VRF protocol fee in native wei.
    /// @dev Reverts during oracle warm-up, stale history or recovery.
    ///      Callback funding is deliberately excluded from this value.
    function quoteProtocolFee() external view returns (uint256 fee) {
        (
            bool valid,
            uint256 deltaPrice0Cumulative,
            uint256 elapsedSeconds
        ) = _billingWindow();

        if (!valid) {
            revert PricingUnavailable();
        }

        uint256 denominator = elapsedSeconds * Q112;

        return _mulDivRoundingUp(
            VRF_BASE_FEE_TRUSD_BASE_UNITS,
            deltaPrice0Cumulative,
            denominator
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

    /// @dev Selects the deterministic TWAP billing window.
    ///      The newest qualifying older observation is the canonical baseline.
    function _billingWindow()
        internal
        view
        returns (
            bool valid,
            uint256 deltaPrice0Cumulative,
            uint256 elapsedSeconds
        )
    {
        uint8 count = _observationCount;

        if (count < 2) {
            return (false, 0, 0);
        }

        Observation storage latest =
            _observations[_observationIndex];

        if (block.timestamp < uint256(latest.observedAt)) {
            return (false, 0, 0);
        }

        if (
            block.timestamp - uint256(latest.observedAt)
                > TWAP_MAX_AGE_SECONDS
        ) {
            return (false, 0, 0);
        }

        for (uint256 offset = 1; offset < uint256(count); offset++) {
            uint256 baselineIndex =
                (
                    uint256(_observationIndex)
                    + TWAP_OBSERVATION_RING_SIZE
                    - offset
                )
                % TWAP_OBSERVATION_RING_SIZE;

            Observation storage baseline =
                _observations[baselineIndex];

            if (baseline.observedAt > latest.observedAt) {
                return (false, 0, 0);
            }

            uint256 elapsed =
                uint256(latest.observedAt)
                - uint256(baseline.observedAt);

            if (elapsed < TWAP_MIN_WINDOW_SECONDS) {
                continue;
            }

            if (block.timestamp < uint256(baseline.observedAt)) {
                return (false, 0, 0);
            }

            if (
                block.timestamp - uint256(baseline.observedAt)
                    > TWAP_MAX_AGE_SECONDS
            ) {
                return (false, 0, 0);
            }

            uint256 delta;

            unchecked {
                delta =
                    latest.price0Cumulative
                    - baseline.price0Cumulative;
            }

            return (true, delta, elapsed);
        }

        return (false, 0, 0);
    }

    /// @dev Full-precision x*y/denominator rounded upward.
    function _mulDivRoundingUp(
        uint256 x,
        uint256 y,
        uint256 denominator
    )
        internal
        pure
        returns (uint256 result)
    {
        unchecked {
            if (denominator == 0) {
                revert FeeOverflow();
            }

            uint256 prod0;
            uint256 prod1;

            assembly {
                let mm := mulmod(x, y, not(0))
                prod0 := mul(x, y)
                prod1 := sub(
                    sub(mm, prod0),
                    lt(mm, prod0)
                )
            }

            uint256 remainder;

            if (prod1 == 0) {
                result = prod0 / denominator;
                remainder = mulmod(x, y, denominator);

                if (remainder != 0) {
                    if (result == type(uint256).max) {
                        revert FeeOverflow();
                    }
                    result += 1;
                }

                return result;
            }

            if (denominator <= prod1) {
                revert FeeOverflow();
            }

            assembly {
                remainder := mulmod(x, y, denominator)
                prod1 := sub(
                    prod1,
                    gt(remainder, prod0)
                )
                prod0 := sub(prod0, remainder)
            }

            uint256 twos =
                denominator & (0 - denominator);

            assembly {
                denominator := div(denominator, twos)
                prod0 := div(prod0, twos)
                twos := add(
                    div(sub(0, twos), twos),
                    1
                )
            }

            prod0 |= prod1 * twos;

            uint256 inverse =
                (3 * denominator) ^ 2;

            inverse *= 2 - denominator * inverse;
            inverse *= 2 - denominator * inverse;
            inverse *= 2 - denominator * inverse;
            inverse *= 2 - denominator * inverse;
            inverse *= 2 - denominator * inverse;
            inverse *= 2 - denominator * inverse;

            result = prod0 * inverse;

            if (remainder != 0) {
                if (result == type(uint256).max) {
                    revert FeeOverflow();
                }
                result += 1;
            }

            return result;
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

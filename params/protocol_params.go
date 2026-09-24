// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

const (
	GasLimitBoundDivisor uint64 = 1024               // The bound divisor of the gas limit, used in update calculations.
	MinGasLimit          uint64 = 5000               // Minimum the gas limit may ever be.
	MaxGasLimit          uint64 = 0x7fffffffffffffff // Maximum the gas limit (2^63-1).
	GenesisGasLimit      uint64 = 4712388            // Gas limit of the Genesis block.

	MaxTxGas uint64 = 1 << 24 // Maximum transaction gas limit after eip-7825 (16,777,216).

	MaximumExtraDataSize  uint64 = 32    // Maximum size extra data may be after Genesis.
	CallValueTransferGas  uint64 = 9000  // Paid for CALL when the value transfer is non-zero.
	CallNewAccountGas     uint64 = 25000 // Paid for CALL when the destination address didn't exist prior.
	TxGas                 uint64 = 21000 // Per transaction not creating a contract. NOTE: Not payable on data of calls between transactions.
	TxGasContractCreation uint64 = 53000 // Per transaction that creates a contract. NOTE: Not payable on data of calls between transactions.
	TxDataZeroGas         uint64 = 4     // Per byte of data attached to a transaction that equals zero. NOTE: Not payable on data of calls between transactions.
	QuadCoeffDiv          uint64 = 512   // Divisor for the quadratic particle of the memory cost equation.
	LogDataGas            uint64 = 8     // Per byte in a LOG* operation's data.
	CallStipend           uint64 = 2300  // Free gas given at beginning of call.

	Keccak256Gas     uint64 = 30 // Once per KECCAK256 operation.
	Keccak256WordGas uint64 = 6  // Once per word of the KECCAK256 operation's data.
	InitCodeWordGas  uint64 = 2  // Once per word of the init code when creating a contract.

	SstoreSetGas    uint64 = 20000 // Once per SSTORE operation.
	SstoreResetGas  uint64 = 5000  // Once per SSTORE operation if the zeroness changes from zero.
	SstoreClearGas  uint64 = 5000  // Once per SSTORE operation if the zeroness doesn't change.
	SstoreRefundGas uint64 = 15000 // Once per SSTORE operation if the zeroness changes to zero.

	NetSstoreNoopGas  uint64 = 200   // Once per SSTORE operation if the value doesn't change.
	NetSstoreInitGas  uint64 = 20000 // Once per SSTORE operation from clean zero.
	NetSstoreCleanGas uint64 = 5000  // Once per SSTORE operation from clean non-zero.
	NetSstoreDirtyGas uint64 = 200   // Once per SSTORE operation from dirty.

	NetSstoreClearRefund      uint64 = 15000 // Once per SSTORE operation for clearing an originally existing storage slot
	NetSstoreResetRefund      uint64 = 4800  // Once per SSTORE operation for resetting to the original non-zero value
	NetSstoreResetClearRefund uint64 = 19800 // Once per SSTORE operation for resetting to the original zero value

	SstoreSentryGasEIP2200            uint64 = 2300  // Minimum gas required to be present for an SSTORE call, not consumed
	SstoreSetGasEIP2200               uint64 = 20000 // Once per SSTORE operation from clean zero to non-zero
	SstoreResetGasEIP2200             uint64 = 5000  // Once per SSTORE operation from clean non-zero to something else
	SstoreClearsScheduleRefundEIP2200 uint64 = 15000 // Once per SSTORE operation for clearing an originally existing storage slot

	ColdAccountAccessCostEIP2929 = uint64(2600) // COLD_ACCOUNT_ACCESS_COST
	ColdSloadCostEIP2929         = uint64(2100) // COLD_SLOAD_COST
	WarmStorageReadCostEIP2929   = uint64(100)  // WARM_STORAGE_READ_COST

	// In EIP-2200: SstoreResetGas was 5000.
	// In EIP-2929: SstoreResetGas was changed to '5000 - COLD_SLOAD_COST'.
	// In EIP-3529: SSTORE_CLEARS_SCHEDULE is defined as SSTORE_RESET_GAS + ACCESS_LIST_STORAGE_KEY_COST
	// Which becomes: 5000 - 2100 + 1900 = 4800
	SstoreClearsScheduleRefundEIP3529 uint64 = SstoreResetGasEIP2200 - ColdSloadCostEIP2929 + TxAccessListStorageKeyGas

	JumpdestGas uint64 = 1 // Once per JUMPDEST operation.

	CreateDataGas         uint64 = 200   //
	CallCreateDepth       uint64 = 1024  // Maximum depth of call/create stack.
	ExpGas                uint64 = 10    // Once per EXP instruction
	LogGas                uint64 = 375   // Per LOG* operation.
	CopyGas               uint64 = 3     //	Multiplied by the number of 32-byte words that are copied (round up) for any *COPY operation and added.
	StackLimit            uint64 = 1024  // Maximum size of VM stack allowed.
	LogTopicGas           uint64 = 375   // Multiplied by the * of the LOG*, per LOG transaction. e.g. LOG0 incurs 0 * c_txLogTopicGas, LOG4 incurs 4 * c_txLogTopicGas.
	CreateGas             uint64 = 32000 // Once per CREATE operation & contract-creation transaction.
	Create2Gas            uint64 = 32000 // Once per CREATE2 operation
	CreateNGasEip4762     uint64 = 1000  // Once per CREATEn operations post-verkle
	SelfdestructRefundGas uint64 = 24000 // Refunded following a selfdestruct operation.
	MemoryGas             uint64 = 3     // Times the address of the (highest referenced byte in memory + 1). NOTE: referencing happens on read, write and in instructions such as RETURN and CALL.

	TxDataNonZeroGasFrontier  uint64 = 68    // Per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.
	TxDataNonZeroGasEIP2028   uint64 = 16    // Per byte of non zero data attached to a transaction after EIP 2028 (part in Istanbul)
	TxTokenPerNonZeroByte     uint64 = 4     // Token cost per non-zero byte as specified by EIP-7623.
	TxCostFloorPerToken       uint64 = 10    // Cost floor per byte of data as specified by EIP-7623.
	TxCostFloorPerToken7976   uint64 = 16    // Cost floor per byte of data as specified by EIP-7976.
	TxAccessListAddressGas    uint64 = 2400  // Per address specified in EIP 2930 access list
	TxAccessListStorageKeyGas uint64 = 1900  // Per storage key specified in EIP 2930 access list
	TxAuthTupleGas            uint64 = 12500 // Per auth tuple code specified in EIP-7702

	// RegularPerAuthBaseCost is the state-independent per-authorization floor,
	// defined in EIP-8037 as the sum of:
	//
	// - Calldata cost for the authorization tuple
	// - ECDSA recovery of the authority address
	// - Cold authority access (COLD_ACCOUNT_ACCESS)
	// - Warm writes to the authority account
	RegularPerAuthBaseCost uint64 = 7816

	// EIP-2780: resource-based intrinsic transaction gas.
	TxBaseCost2780      uint64 = 12000
	TxValueCost2780     uint64 = 4244
	TransferLogCost2780 uint64 = 1756

	// EIP-8038: state-access gas cost update (Amsterdam).
	ColdAccountAccessAmsterdam         uint64 = 3000  // COLD_ACCOUNT_ACCESS: cold touch of an account
	WarmAccountAccessAmsterdam         uint64 = 100   // WARM_ACCESS: warm touch of an account
	AccountWriteAmsterdam              uint64 = 8000  // ACCOUNT_WRITE: surcharge for first-time write to an account
	CallValueTransferAmsterdam         uint64 = 10300 // CALL_VALUE = ACCOUNT_WRITE + CallStipend (2300)
	ColdStorageAccessAmsterdam         uint64 = 3000  // COLD_STORAGE_ACCESS: cold touch of a storage slot
	WarmStorageAccessAmsterdam         uint64 = 100   // WARM_STORAGE_ACCESS: warm touch of a storage slot
	StorageWriteAmsterdam              uint64 = 10000 // STORAGE_WRITE: surcharge for first-time write to a storage slot
	StorageClearRefundAmsterdam        uint64 = 12480 // STORAGE_CLEAR_REFUND: refund for clearing a storage slot
	CreateAccessAmsterdam              uint64 = 11000 // CREATE_ACCESS = ACCOUNT_WRITE + COLD_STORAGE_ACCESS
	TxAccessListAddressGasAmsterdam    uint64 = 3000  // ACCESS_LIST_ADDRESS_COST
	TxAccessListStorageKeyGasAmsterdam uint64 = 3000  // ACCESS_LIST_STORAGE_KEY_COST

	// These have been changed during the course of the chain
	CallGasFrontier              uint64 = 40  // Once per CALL operation & message call transaction.
	CallGasEIP150                uint64 = 700 // Static portion of gas for CALL-derivates after EIP 150 (Tangerine)
	BalanceGasFrontier           uint64 = 20  // The cost of a BALANCE operation
	BalanceGasEIP150             uint64 = 400 // The cost of a BALANCE operation after Tangerine
	BalanceGasEIP1884            uint64 = 700 // The cost of a BALANCE operation after EIP 1884 (part of Istanbul)
	ExtcodeSizeGasFrontier       uint64 = 20  // Cost of EXTCODESIZE before EIP 150 (Tangerine)
	ExtcodeSizeGasEIP150         uint64 = 700 // Cost of EXTCODESIZE after EIP 150 (Tangerine)
	SloadGasFrontier             uint64 = 50
	SloadGasEIP150               uint64 = 200
	SloadGasEIP1884              uint64 = 800  // Cost of SLOAD after EIP 1884 (part of Istanbul)
	SloadGasEIP2200              uint64 = 800  // Cost of SLOAD after EIP 2200 (part of Istanbul)
	ExtcodeHashGasConstantinople uint64 = 400  // Cost of EXTCODEHASH (introduced in Constantinople)
	ExtcodeHashGasEIP1884        uint64 = 700  // Cost of EXTCODEHASH after EIP 1884 (part in Istanbul)
	SelfdestructGasEIP150        uint64 = 5000 // Cost of SELFDESTRUCT post EIP 150 (Tangerine)

	// EXP has a dynamic portion depending on the size of the exponent
	ExpByteFrontier uint64 = 10 // was set to 10 in Frontier
	ExpByteEIP158   uint64 = 50 // was raised to 50 during Eip158 (Spurious Dragon)

	// Extcodecopy has a dynamic AND a static cost. This represents only the
	// static portion of the gas. It was changed during EIP 150 (Tangerine)
	ExtcodeCopyBaseFrontier uint64 = 20
	ExtcodeCopyBaseEIP150   uint64 = 700

	// CreateBySelfdestructGas is used when the refunded account is one that does
	// not exist. This logic is similar to call.
	// Introduced in Tangerine Whistle (Eip 150)
	CreateBySelfdestructGas uint64 = 25000

	DefaultBaseFeeChangeDenominator = 8          // Bounds the amount the base fee can change between blocks.
	DefaultElasticityMultiplier     = 2          // Bounds the maximum gas limit an EIP-1559 block may have.
	InitialBaseFee                  = 1000000000 // Initial base fee for EIP-1559 blocks.

	MaxCodeSize              = 24576                    // Maximum bytecode to permit for a contract
	MaxInitCodeSize          = 2 * MaxCodeSize          // Maximum initcode to permit in a creation transaction and create instructions
	MaxCodeSizeAmsterdam     = 65536                    // Maximum bytecode to permit for a contract post Amsterdam
	MaxInitCodeSizeAmsterdam = 2 * MaxCodeSizeAmsterdam // Maximum initcode to permit in a creation transaction and create instructions post Amsterdam

	// Precompiled contract gas prices

	EcrecoverGas        uint64 = 3000 // Elliptic curve sender recovery gas price
	Sha256BaseGas       uint64 = 60   // Base price for a SHA256 operation
	Sha256PerWordGas    uint64 = 12   // Per-word price for a SHA256 operation
	Ripemd160BaseGas    uint64 = 600  // Base price for a RIPEMD160 operation
	Ripemd160PerWordGas uint64 = 120  // Per-word price for a RIPEMD160 operation
	IdentityBaseGas     uint64 = 15   // Base price for a data copy operation
	IdentityPerWordGas  uint64 = 3    // Per-work price for a data copy operation

	Bn256AddGasByzantium             uint64 = 500    // Byzantium gas needed for an elliptic curve addition
	Bn256AddGasIstanbul              uint64 = 150    // Gas needed for an elliptic curve addition
	Bn256ScalarMulGasByzantium       uint64 = 40000  // Byzantium gas needed for an elliptic curve scalar multiplication
	Bn256ScalarMulGasIstanbul        uint64 = 6000   // Gas needed for an elliptic curve scalar multiplication
	Bn256PairingBaseGasByzantium     uint64 = 100000 // Byzantium base price for an elliptic curve pairing check
	Bn256PairingBaseGasIstanbul      uint64 = 45000  // Base price for an elliptic curve pairing check
	Bn256PairingPerPointGasByzantium uint64 = 80000  // Byzantium per-point price for an elliptic curve pairing check
	Bn256PairingPerPointGasIstanbul  uint64 = 34000  // Per-point price for an elliptic curve pairing check

	Bls12381G1AddGas          uint64 = 375   // Price for BLS12-381 elliptic curve G1 point addition
	Bls12381G1MulGas          uint64 = 12000 // Price for BLS12-381 elliptic curve G1 point scalar multiplication
	Bls12381G2AddGas          uint64 = 600   // Price for BLS12-381 elliptic curve G2 point addition
	Bls12381G2MulGas          uint64 = 22500 // Price for BLS12-381 elliptic curve G2 point scalar multiplication
	Bls12381PairingBaseGas    uint64 = 37700 // Base gas price for BLS12-381 elliptic curve pairing check
	Bls12381PairingPerPairGas uint64 = 32600 // Per-point pair gas price for BLS12-381 elliptic curve pairing check
	Bls12381MapG1Gas          uint64 = 5500  // Gas price for BLS12-381 mapping field element to G1 operation
	Bls12381MapG2Gas          uint64 = 23800 // Gas price for BLS12-381 mapping field element to G2 operation

	P256VerifyGas uint64 = 6900 // secp256r1 elliptic curve signature verifier gas price

	// The Refund Quotient is the cap on how much of the used gas can be refunded. Before EIP-3529,
	// up to half the consumed gas could be refunded. Redefined as 1/5th in EIP-3529
	RefundQuotient        uint64 = 2
	RefundQuotientEIP3529 uint64 = 5

	BlobTxBytesPerFieldElement         = 32      // Size in bytes of a field element
	BlobTxFieldElementsPerBlob         = 4096    // Number of field elements stored in a single data blob
	BlobTxBlobGasPerBlob               = 1 << 17 // Gas consumption of a single data blob (== blob byte size)
	BlobTxMinBlobGasprice              = 1       // Minimum gas price for data blobs
	BlobTxPointEvaluationPrecompileGas = 50000   // Gas price for the point evaluation precompile.
	BlobTxMaxBlobs                     = 6
	BlobBaseCost                       = 1 << 13 // Base execution gas cost for a blob.

	HistoryServeWindow = 8191 // Number of blocks to serve historical block hashes for, EIP-2935.

	MaxBlockSize = 8_388_608 // maximum size of an RLP-encoded block

	// BALItemCost is the gas-cost divisor for the EIP-7928 block access list
	// size constraint: bal_items <= block_gas_limit / BALItemCost, where
	// bal_items counts every distinct address in the BAL plus every storage
	// key (writes + reads) carried by those accounts.
	//
	// The value (2000) is set deliberately below COLD_SLOAD_COST (2100) so
	// the bound has a small safety margin for system-contract accesses that
	// don't consume block gas.
	BALItemCost uint64 = 2000

	AccountCreationSize       = 120
	StorageCreationSize       = 64
	AuthorizationCreationSize = 23
	CostPerStateByte          = 1530
	SystemMaxSStoresPerCall   = 16
)

// Bls12381G1MultiExpDiscountTable is the gas discount table for BLS12-381 G1 multi exponentiation operation
var Bls12381G1MultiExpDiscountTable = [128]uint64{1000, 949, 848, 797, 764, 750, 738, 728, 719, 712, 705, 698, 692, 687, 682, 677, 673, 669, 665, 661, 658, 654, 651, 648, 645, 642, 640, 637, 635, 632, 630, 627, 625, 623, 621, 619, 617, 615, 613, 611, 609, 608, 606, 604, 603, 601, 599, 598, 596, 595, 593, 592, 591, 589, 588, 586, 585, 584, 582, 581, 580, 579, 577, 576, 575, 574, 573, 572, 570, 569, 568, 567, 566, 565, 564, 563, 562, 561, 560, 559, 558, 557, 556, 555, 554, 553, 552, 551, 550, 549, 548, 547, 547, 546, 545, 544, 543, 542, 541, 540, 540, 539, 538, 537, 536, 536, 535, 534, 533, 532, 532, 531, 530, 529, 528, 528, 527, 526, 525, 525, 524, 523, 522, 522, 521, 520, 520, 519}

// Bls12381G2MultiExpDiscountTable is the gas discount table for BLS12-381 G2 multi exponentiation operation
var Bls12381G2MultiExpDiscountTable = [128]uint64{1000, 1000, 923, 884, 855, 832, 812, 796, 782, 770, 759, 749, 740, 732, 724, 717, 711, 704, 699, 693, 688, 683, 679, 674, 670, 666, 663, 659, 655, 652, 649, 646, 643, 640, 637, 634, 632, 629, 627, 624, 622, 620, 618, 615, 613, 611, 609, 607, 606, 604, 602, 600, 598, 597, 595, 593, 592, 590, 589, 587, 586, 584, 583, 582, 580, 579, 578, 576, 575, 574, 573, 571, 570, 569, 568, 567, 566, 565, 563, 562, 561, 560, 559, 558, 557, 556, 555, 554, 553, 552, 552, 551, 550, 549, 548, 547, 546, 545, 545, 544, 543, 542, 541, 541, 540, 539, 538, 537, 537, 536, 535, 535, 534, 533, 532, 532, 531, 530, 530, 529, 528, 528, 527, 526, 526, 525, 524, 524}

// Difficulty parameters.
var (
	DifficultyBoundDivisor = big.NewInt(2048)   // The bound divisor of the difficulty, used in the update calculations.
	GenesisDifficulty      = big.NewInt(131072) // Difficulty of the Genesis block.
	MinimumDifficulty      = big.NewInt(131072) // The minimum that the difficulty may ever be.
	DurationLimit          = big.NewInt(13)     // The decision boundary on the blocktime duration used to determine whether difficulty should go up or not.
)

// System contracts.
var (
	// SystemAddress is where the system-transaction is sent from as per EIP-4788
	SystemAddress = common.HexToAddress("0xfffffffffffffffffffffffffffffffffffffffe")

	// EIP-4788 - Beacon block root in the EVM
	BeaconRootsAddress = common.HexToAddress("0x000F3df6D732807Ef1319fB7B8bB8522d0Beac02")
	BeaconRootsCode    = common.FromHex("3373fffffffffffffffffffffffffffffffffffffffe14604d57602036146024575f5ffd5b5f35801560495762001fff810690815414603c575f5ffd5b62001fff01545f5260205ff35b5f5ffd5b62001fff42064281555f359062001fff015500")

	// EIP-2935 - Serve historical block hashes from state
	HistoryStorageAddress = common.HexToAddress("0x0000F90827F1C53a10cb7A02335B175320002935")
	HistoryStorageCode    = common.FromHex("3373fffffffffffffffffffffffffffffffffffffffe14604657602036036042575f35600143038111604257611fff81430311604257611fff9006545f5260205ff35b5f5ffd5b5f35611fff60014303065500")

	// EIP-7002 - Execution layer triggerable withdrawals
	WithdrawalQueueAddress = common.HexToAddress("0x00000961Ef480Eb55e80D19ad83579A64c007002")
	WithdrawalQueueCode    = common.FromHex("3373fffffffffffffffffffffffffffffffffffffffe1460cb5760115f54807fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff146101f457600182026001905f5b5f82111560685781019083028483029004916001019190604d565b909390049250505036603814608857366101f457346101f4575f5260205ff35b34106101f457600154600101600155600354806003026004013381556001015f35815560010160203590553360601b5f5260385f601437604c5fa0600101600355005b6003546002548082038060101160df575060105b5f5b8181146101835782810160030260040181604c02815460601b8152601401816001015481526020019060020154807fffffffffffffffffffffffffffffffff00000000000000000000000000000000168252906010019060401c908160381c81600701538160301c81600601538160281c81600501538160201c81600401538160181c81600301538160101c81600201538160081c81600101535360010160e1565b910180921461019557906002556101a0565b90505f6002555f6003555b5f54807fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff14156101cd57505f5b6001546002828201116101e25750505f6101e8565b01600290035b5f555f600155604c025ff35b5f5ffd")

	// EIP-7251 - Increase the MAX_EFFECTIVE_BALANCE
	ConsolidationQueueAddress = common.HexToAddress("0x0000BBdDc7CE488642fb579F8B00f3a590007251")
	ConsolidationQueueCode    = common.FromHex("3373fffffffffffffffffffffffffffffffffffffffe1460d35760115f54807fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff1461019a57600182026001905f5b5f82111560685781019083028483029004916001019190604d565b9093900492505050366060146088573661019a573461019a575f5260205ff35b341061019a57600154600101600155600354806004026004013381556001015f358155600101602035815560010160403590553360601b5f5260605f60143760745fa0600101600355005b6003546002548082038060021160e7575060025b5f5b8181146101295782810160040260040181607402815460601b815260140181600101548152602001816002015481526020019060030154905260010160e9565b910180921461013b5790600255610146565b90505f6002555f6003555b5f54807fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff141561017357505f5b6001546001828201116101885750505f61018e565b01600190035b5f555f6001556074025ff35b5f5ffd")

	// EIP-8282 - Builder Execution Requests
	BuilderDepositAddress = common.HexToAddress("0x0000BFF46984E3725691FA540A8C7589300D8282")
	BuilderDepositCode    = common.FromHex("0x3373fffffffffffffffffffffffffffffffffffffffe1461011c575f54807fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff146102705760015460088111605257506058565b60089003015b601190600182026001905f5b5f821115607f57810190830284830290049160010191906064565b90939004925050503660b814609f57366102705734610270575f5260205ff35b8034106102705760383567ffffffffffffffff1680633b9aca001161027057633b9aca00029034031061027057600154600101600155600354806006026004015f358155600101602035815560010160403581556001016060358155600101608035815560010160a035905560b85f5f3760b85fa0600101600355005b60035460025480820380604011610131575060405b5f5b8181146101d7578281016006026004018160b8028154815260200181600101548152602001816002015480825260401c67ffffffffffffffff16816010018160381c81600701538160301c81600601538160281c81600501538160201c81600401538160181c81600301538160101c81600201538160081c816001015353602001816003015481526020018160040154815260200190600501549052600101610133565b91018092146101e957906002556101f4565b90505f6002555f6003555b36610242575f54600154817fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff1461023057600882820111610238575b50505f610264565b0160089003610264565b7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff5b5f555f60015560b8025ff35b5f5ffd")
	BuilderExitAddress    = common.HexToAddress("0x000064D678505AD48F8CCB093BC65613800E8282")
	BuilderExitCode       = common.FromHex("0x3373fffffffffffffffffffffffffffffffffffffffe1460e1575f54807fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff146101c65760015460028111605157506057565b60029003015b601190600182026001905f5b5f821115607e57810190830284830290049160010191906063565b909390049250505036603014609e57366101c657346101c6575f5260205ff35b34106101c657600154600101600155600354806003026004013381556001015f35815560010160203590553360601b5f5260305f60143760445fa0600101600355005b6003546002548082038060101160f5575060105b5f5b81811461012d5782810160030260040181604402815460601b8152601401816001015481526020019060020154905260010160f7565b910180921461013f579060025561014a565b90505f6002555f6003555b36610198575f54600154817fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff146101865760028282011161018e575b50505f6101ba565b01600290036101ba565b7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff5b5f555f6001556044025ff35b5f5ffd")

	// Rabbit VRF Coordinator V1 - consensus-installed EVM facade.
	RabbitVRFCoordinatorV1Address = common.HexToAddress("0xdFc21aeA108e3F527E5f236ebf354dc8262719da")
	RabbitVRFCoordinatorV1Code    = common.FromHex("0x6080604052600436106101145760003560e01c80638c75d76a116100a0578063dfee8cd611610064578063dfee8cd61461033d578063f641afcc14610353578063f6d4fd1b1461037b578063fb1e61ca1461038e578063ffa1ad74146104a557600080fd5b80638c75d76a146102bf5780638e4900f5146102d4578063ba45c532146102ea578063c80c39f314610300578063d6cf0c371461032057600080fd5b80634e91fa73116100e75780634e91fa73146101ca57806358f6b1fb146101e0578063614f5471146102085780638609dbd51461022e5780638be6da2e1461028557600080fd5b806305e61835146101195780633434735f1461014157806339d5c96b146101745780634b7dec701461018b575b600080fd5b34801561012557600080fd5b5061012e6104ba565b6040519081526020015b60405180910390f35b34801561014d57600080fd5b5061015c6002600160a01b0381565b6040516001600160a01b039091168152602001610138565b34801561018057600080fd5b506101896104c9565b005b34801561019757600080fd5b506101ab6101a636600461120d565b6106d8565b604080516001600160401b039093168352602083019190915201610138565b3480156101d657600080fd5b5061012e6103e881565b3480156101ec57600080fd5b5061015c73dfc21aea108e3f527e5f236ebf354dc8262719da81565b34801561021457600080fd5b5060005460ff165b60405160ff9091168152602001610138565b34801561023a57600080fd5b5061026d610249366004611230565b6001600160a01b03166000908152602160205260409020546001600160401b031690565b6040516001600160401b039091168152602001610138565b34801561029157600080fd5b5061029a610738565b6040805193151584526001600160401b03909216602084015290820152606001610138565b3480156102cb57600080fd5b5061012e601081565b3480156102e057600080fd5b5061012e610e1081565b3480156102f657600080fd5b5061012e61070881565b34801561030c57600080fd5b5061012e61031b366004611272565b610797565b34801561032c57600080fd5b50600054610100900460ff1661021c565b34801561034957600080fd5b5061012e61012c81565b34801561035f57600080fd5b5061015c738b9f4581b71964049ac6be03b22000132438b38581565b61012e61038936600461128d565b6107cc565b34801561039a57600080fd5b5061042e6103a93660046112b7565b60009081526022602052604090208054600182015460028301546003840154600485015460058601546006909601546001600160a01b03861697600160a01b9096046001600160401b039081169781871697680100000000000000008804831697600160801b810490931696600160c01b90930463ffffffff16959294929360ff1690565b604080516001600160a01b03909c168c526001600160401b039a8b1660208d0152988a16988b019890985295881660608a015296909316608088015263ffffffff90911660a087015260c086015260e085015261010084019290925261012083019190915260ff1661014082015261016001610138565b3480156104b157600080fd5b5061012e600181565b60006104c4610bff565b905090565b336002600160a01b03146104f0576040516329fa754760e01b815260040160405180910390fd5b6000806104fb610c59565b9150915081610508575050565b6001600160401b0342111561051b575050565b60008054429160ff909116900361057857604080518082019091526001600160401b039190911680825260209091018290526001805467ffffffffffffffff191690911781556002919091556000805461ffff1916909117905550565b60008054600190610100900460ff1660108110610597576105976112d0565b6002020180549091506001600160401b0390811690831610156105ba5750505050565b805461012c906105d3906001600160401b0316846112fc565b6001600160401b031610156105e85750505050565b6000805460109061060290610100900460ff16600161131b565b61060c9190611344565b90506040518060400160405280846001600160401b031681526020018581525060018260ff1660108110610642576106426112d0565b82516002919091029190910180546001600160401b0390921667ffffffffffffffff199092169190911781556020909101516001909101556000805460ff8084166101000261ff001983168117909355601092811691161710156106d157600080546001919081906106b890849060ff16611366565b92506101000a81548160ff021916908360ff1602179055505b5050505050565b60008060108360ff16106106ff57604051633f2228a560e21b815260040160405180910390fd5b600060018460ff1660108110610717576107176112d0565b6002020180546001909101546001600160401b039091169590945092505050565b600080548190819060ff1681036107555750600092839250829150565b60008054600190610100900460ff1660108110610774576107746112d0565b60020201805460019182015491966001600160401b039091169550909350915050565b600063ffffffff8216156107be576040516316cf90a160e11b815260040160405180910390fd5b6107c6610bff565b92915050565b600063ffffffff8316156107f3576040516316cf90a160e11b815260040160405180910390fd5b6001600160401b0343111561081b57604051633582cd6160e11b815260040160405180910390fd5b6000610825610bff565b9050803414610855576040516339bdab8360e11b8152600481018290523460248201526044015b60405180910390fd5b336000908152602160205260409020546001600160401b031667fffffffffffffffe1981016108995760405163817b579960e01b815233600482015260240161084c565b604080517f122b6196b247131ff9ec6a451974708343b9fb2f469ec6f64e1afe9c55be14f0602082015246918101919091523060608201523360808201526001600160401b03821660a082015263ffffffff861660c082015260e081018590526101000160408051601f1981840301815291815281516020928301206000818152602290935291206006015490935060ff161561094c576040516376f74a2d60e11b81526004810184905260240161084c565b604051806101600160405280336001600160a01b03168152602001826001600160401b03168152602001436001600160401b0316815260200160006001600160401b0316815260200160006001600160401b031681526020018663ffffffff1681526020018381526020018581526020016000801b81526020016000801b8152602001600160ff168152506022600085815260200190815260200160002060008201518160000160006101000a8154816001600160a01b0302191690836001600160a01b0316021790555060208201518160000160146101000a8154816001600160401b0302191690836001600160401b0316021790555060408201518160010160006101000a8154816001600160401b0302191690836001600160401b0316021790555060608201518160010160086101000a8154816001600160401b0302191690836001600160401b0316021790555060808201518160010160106101000a8154816001600160401b0302191690836001600160401b0316021790555060a08201518160010160186101000a81548163ffffffff021916908363ffffffff16021790555060c0820151816002015560e08201518160030155610100820151816004015561012082015181600501556101408201518160060160006101000a81548160ff021916908360ff1602179055509050508060010160216000336001600160a01b03166001600160a01b0316815260200190815260200160002060006101000a8154816001600160401b0302191690836001600160401b03160217905550806001600160401b0316336001600160a01b0316847f9269d882c9ec0d8d6045606c49bc061a519c0f5ad66c9f1d6d620daf1b79f7ac888887604051610bef9392919063ffffffff9390931683526020830191909152604082015260600190565b60405180910390a4505092915050565b600080600080610c0d610ecc565b92509250925082610c315760405163ec06ade960e01b815260040160405180910390fd5b6000610c41600160701b8361137f565b9050610c506103e884836110aa565b94505050505090565b60408051600481526024810182526020810180516001600160e01b0316635909c0d560e01b1790529051600091829182918291738b9f4581b71964049ac6be03b22000132438b38591610cab91611396565b600060405180830381855afa9150503d8060008114610ce6576040519150601f19603f3d011682016040523d82523d6000602084013e610ceb565b606091505b5091509150811580610cff57508051602014155b15610d105750600093849350915050565b602081810151604080516004815260248101825292830180516001600160e01b0316630240bc6b60e21b1790525190916000918291738b9f4581b71964049ac6be03b22000132438b38591610d659190611396565b600060405180830381855afa9150503d8060008114610da0576040519150601f19603f3d011682016040523d82523d6000602084013e610da5565b606091505b5091509150811580610db957508051606014155b15610dcd5750600096879650945050505050565b6020810151604082015160608301516001600160701b03831180610df757506001600160701b0382115b80610e05575063ffffffff81115b15610e1c57506000998a9950975050505050505050565b8282826001600160701b0383161580610e3c57506001600160701b038216155b15610e56575060009c8d9c509a5050505050505050505050565b4281810363ffffffff8116600003610e7f575060019e999d50989b505050505050505050505050565b6000856001600160701b03166070866001600160701b0316901b81610ea657610ea661132e565b0490508163ffffffff1681028c019e50505060019d505050505050505050505050509091565b600080548190819060ff166002811015610eee57506000938493508392509050565b60008054600190610100900460ff1660108110610f0d57610f0d6112d0565b6002020180549091506001600160401b0316421015610f355750600094859450849350915050565b8054610e1090610f4e906001600160401b0316426113c5565b1115610f635750600094859450849350915050565b60015b8260ff1681101561109a57600080546010908390610f8d908390610100900460ff1661131b565b610f9791906113c5565b610fa19190611344565b9050600060018260108110610fb857610fb86112d0565b85546002919091029190910180549092506001600160401b0391821691161115610fee5750600097889750879650945050505050565b8054845460009161100b916001600160401b0391821691166113c5565b905061070881101561101f57505050611092565b81546001600160401b0316421015611044575060009889985088975095505050505050565b8154610e109061105d906001600160401b0316426113c5565b1115611076575060009889985088975095505050505050565b6001918201549482015491999490910397509550919350505050565b600101610f66565b5060009586955085945092505050565b6000816000036110cd576040516340c0d6e560e11b815260040160405180910390fd5b60008060001985870985870292508281108382030391505060008160000361114b578483816110fe576110fe61132e565b049350848061110f5761110f61132e565b8688099050801561114357600019840361113c576040516340c0d6e560e11b815260040160405180910390fd5b6001840193505b505050611206565b81851161116b576040516340c0d6e560e11b815260040160405180910390fd5b848688096000868103871696879004966002600389028118808a02820302808a02820302808a02820302808a02820302808a02820302808a0290910302918190038190046001018684119095038581029684900391909104959095178181029650949391925082156112005760001986036111f9576040516340c0d6e560e11b815260040160405180910390fd5b6001860195505b50505050505b9392505050565b60006020828403121561121f57600080fd5b813560ff8116811461120657600080fd5b60006020828403121561124257600080fd5b81356001600160a01b038116811461120657600080fd5b803563ffffffff8116811461126d57600080fd5b919050565b60006020828403121561128457600080fd5b61120682611259565b600080604083850312156112a057600080fd5b6112a983611259565b946020939093013593505050565b6000602082840312156112c957600080fd5b5035919050565b634e487b7160e01b600052603260045260246000fd5b634e487b7160e01b600052601160045260246000fd5b6001600160401b0382811682821603908111156107c6576107c66112e6565b808201808211156107c6576107c66112e6565b634e487b7160e01b600052601260045260246000fd5b60008261136157634e487b7160e01b600052601260045260246000fd5b500690565b60ff81811683821601908111156107c6576107c66112e6565b80820281158282048414176107c6576107c66112e6565b6000825160005b818110156113b7576020818601810151858301520161139d565b506000920191825250919050565b818103818111156107c6576107c66112e656")

	// EIP-7997 - Deterministic deployment factory (keyless CREATE2 factory)
	DeterministicFactoryAddress = common.HexToAddress("0x4e59b44847b379578588920cA78FbF26c0B4956C")
	DeterministicFactoryCode    = common.FromHex("0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe03601600081602082378035828234f58015156039578182fd5b8082525050506014600cf3")
)

// System log events.
var (
	// EIP-7708 - System logs emitted for ETH transfer and burn
	EthTransferLogEvent = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef") // keccak256('Transfer(address,address,uint256)')
	EthBurnLogEvent     = common.HexToHash("0xcc16f5dbb4873280815c1ee09dbd06736cffcc184412cf7a71a0fdb75d397ca5") // keccak256('Burn(address,uint256)')

)

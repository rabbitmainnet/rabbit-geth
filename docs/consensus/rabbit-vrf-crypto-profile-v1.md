# Rabbit VRF Cryptographic Profile V1

Status: BYTE-FROZEN / PROTOCOL NOT YET ACTIVATABLE

This document freezes the byte-level cryptographic profile used by Rabbit VRF
V1. Freezing this profile does not authorize Testnet or Mainnet activation.

## Profile identifier

ASCII:

    RABBIT-VRF-BLS12381-G1PK-G2SIG-V1

## BLS profile

- Curve: BLS12-381.
- Secret scalar field: Fr.
- Public keys: G1.
- Signatures and threshold partials: G2.
- Secret encoding: exactly 32-byte canonical big-endian Fr.
- Zero secret scalar is invalid.
- Values greater than or equal to the Fr modulus are invalid.
- Public key encoding: exactly 48-byte compressed G1.
- Signature encoding: exactly 96-byte compressed G2.
- Infinity is invalid.
- Non-curve points are invalid.
- Wrong-subgroup points are invalid.

## HashToG2

Exact ASCII DST:

    RABBIT-VRF-BLS12381G2_XMD:SHA-256_SSWU_RO_V1

DST length:

    44 bytes

DST hexadecimal:

    5241424249542d5652462d424c53313233383147325f584d443a5348412d3235365f535357555f524f5f5631

The exact message bytes are passed to HashToG2 using this exact DST.

Changing any byte creates a different protocol version.

## Randomness derivation

Exact ASCII domain:

    RABBIT-VRF-RANDOMNESS-V1

Domain length:

    24 bytes

Domain hexadecimal:

    5241424249542d5652462d52414e444f4d4e4553532d5631

After successful signature verification:

    randomness =
        Keccak256(
            ASCII("RABBIT-VRF-RANDOMNESS-V1") ||
            message ||
            compressed_signature_96
        )

There is no separator and no length prefix at this primitive layer.

This construction is unambiguous because the domain is fixed by protocol and
the compressed signature has a fixed 96-byte length. The message itself MUST
later be a canonical Rabbit VRF round message defined by the consensus
protocol.

## Base vector 1

Name:

    VECTOR_1_SK_1_ASCII

Secret:

    0000000000000000000000000000000000000000000000000000000000000001

Message:

    5241424249542d5652462d544553542d564543544f522d31

Public key:

    97f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb

Signature:

    8644d2af9c0ea1954ebaf81f4eb7f4b8055661d08d25819c4dff87adb4b85129339cc2b4c708c03eef3300c755c678a103bcd186066d9f68595185f83576b27faad61b21ce37d0711a87825082bad41a868b6830e23f9486c31a58942c0c5b7f

Randomness:

    2edc66d7e724e9b5313ae6ce7f4a9c6b3df19d3c36039a2468948426ab6e09b2

## Base vector 2

Name:

    VECTOR_2_SK_42_ASCII

Secret:

    000000000000000000000000000000000000000000000000000000000000002a

Message:

    7261626269742d7672662d7468726573686f6c642d6f726465722d746573742d7631

Public key:

    8ce3b57b791798433fd323753489cac9bca43b98deaafaed91f4cb010730ae1e38b186ccd37a09b8aed62ce23b699c48

Signature:

    aa1dd236779293aa6128e3f7d4e5bf251d1f168c9cb7e0da60d5fc65c6867d6d91009b5f91c6e390c53054baf5edf8170b0285fe33efb052ea4ae9236a831d9bd57ba3970c66e84c69770db4aede3b127e2c6df6ba83a7eb1612c3812189902c

Randomness:

    d985ddc0341b6045edf1b3621ef612747591a09df45207152a3991e3e2655a35

## Base vector 3

Name:

    VECTOR_3_SK_Q_MINUS_1_BINARY

Secret:

    73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000000

Message:

    00010203040506070809fafbfcfdfeff80

Public key:

    b7f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb

Signature:

    8d302eb908567402bbc0f9bb619588f7fe979fa56782b66d7e480b4687ca234e03b23c8514a60cfa7b60c5a9ed1261b9056e374d2dc3354beeacae97927fa602684795924a560b6ca698667f2571d58d0b3fd3c8c66ac0a6a673d358c8c40ac3

Randomness:

    0b29100ff3edc36549585f05bca234c37bae0c0707ea590bbe8f55be8f654310

The exact Fr modulus itself MUST be rejected:

    73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001

## Threshold interoperability vector

This polynomial is TEST ONLY and MUST NOT be used as a production dealer:

    f(x) = 42 + 7x + 11x^2

Threshold:

    3

Conceptual test committee size:

    5

Message:

    7261626269742d7672662d7468726573686f6c642d6f726465722d746573742d7631

Master public key:

    8ce3b57b791798433fd323753489cac9bca43b98deaafaed91f4cb010730ae1e38b186ccd37a09b8aed62ce23b699c48

### Share 1

Secret share:

    000000000000000000000000000000000000000000000000000000000000003c

Verification public key:

    b783a70a1cf9f53e7d2ddf386bea81a947e5360c5f1e0bf004fceedb2073e4dd180ef3d2d91bee7b1c5a88d1afd11c49

Partial signature:

    b756ad58635ed9522390a916a6080890c0a9b7979d5c98978b19129f57a8436398f6e6e82410e1d2339f73300e38d02411d4359d529383c3fd2c45ba1fd47406530a1851bda8f63037c494abb742760b24347181ee5e1b76637ae4f954417054

### Share 3

Secret share:

    00000000000000000000000000000000000000000000000000000000000000a2

Verification public key:

    93b15273200e99dbbf91b24f87daa9079a023ccdf4debf84d2f9d0c2a1bf57d3b13591b62b1c513ec08ad20feb011875

Partial signature:

    a3decedd4618b01379c8989249b811f97a9b4038f386dc4c4a80641a19b38ec602d35020d3523d7bb8e1bdf7982fd8a709a80ef525ffa9d1259027928d89f795a5e064d158d84b0f5e28566550763140b929c505cfc03215938cda7058ee123f

### Share 5

Secret share:

    0000000000000000000000000000000000000000000000000000000000000160

Verification public key:

    ac3093600c7c45716cb9baba36022b1c0f93714196f91ea6054fd1d0361e981d041368afa44d9e8ad41a83d3b710284e

Partial signature:

    84cd55bda23a4e26cc08313f6727375155f56f3dbe12d49073019dcb563c251242a6c449630a20e04626bdf4ae8eea1b05be6f81b0bf492493843e640d8107d0dc6352e374e3e5190fa40d33a52e30d50451ae68d2f064d84344c148bfea7956

### Reconstruction

Input ShareIDs:

    5,1,3

Canonical selected ShareIDs:

    1,3,5

Threshold signature:

    aa1dd236779293aa6128e3f7d4e5bf251d1f168c9cb7e0da60d5fc65c6867d6d91009b5f91c6e390c53054baf5edf8170b0285fe33efb052ea4ae9236a831d9bd57ba3970c66e84c69770db4aede3b127e2c6df6ba83a7eb1612c3812189902c

Direct master signature:

    aa1dd236779293aa6128e3f7d4e5bf251d1f168c9cb7e0da60d5fc65c6867d6d91009b5f91c6e390c53054baf5edf8170b0285fe33efb052ea4ae9236a831d9bd57ba3970c66e84c69770db4aede3b127e2c6df6ba83a7eb1612c3812189902c

Randomness:

    d985ddc0341b6045edf1b3621ef612747591a09df45207152a3991e3e2655a35

The threshold signature MUST equal the direct master signature byte-for-byte.

## Executable conformance

The canonical executable vectors are enforced by:

    crypto/rabbitvrf/vectors_test.go

Any implementation claiming Rabbit VRF V1 compatibility MUST reproduce every
public key, signature, partial signature, threshold signature and randomness
value in this document exactly.

Cross-platform and independent-implementation execution are still required
before public protocol activation.

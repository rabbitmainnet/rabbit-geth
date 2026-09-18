package rabbitvrf

import (
	"encoding/hex"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

type cryptoVectorV1 struct {
	name       string
	secret     string
	message    string
	publicKey  string
	signature  string
	randomness string
}

func mustDecodeHexVector(
	t *testing.T,
	value string,
) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("invalid test vector hex: %v", err)
	}

	return decoded
}

func mustFrVector(
	t *testing.T,
	encodedHex string,
) fr.Element {
	t.Helper()

	encoded := mustDecodeHexVector(
		t,
		encodedHex,
	)

	if len(encoded) != SecretKeySize {
		t.Fatalf(
			"invalid Fr vector length: %d",
			len(encoded),
		)
	}

	var scalar fr.Element

	if err := scalar.SetBytesCanonical(
		encoded,
	); err != nil {
		t.Fatalf(
			"invalid canonical Fr vector: %v",
			err,
		)
	}

	return scalar
}

func TestCryptoProfileV1Constants(t *testing.T) {
	if ProfileV1 !=
		"RABBIT-VRF-BLS12381-G1PK-G2SIG-V1" {
		t.Fatalf(
			"ProfileV1 changed: %q",
			ProfileV1,
		)
	}

	if HashToG2DSTV1 !=
		"RABBIT-VRF-BLS12381G2_XMD:SHA-256_SSWU_RO_V1" {
		t.Fatalf(
			"HashToG2DSTV1 changed: %q",
			HashToG2DSTV1,
		)
	}

	if RandomnessDomainV1 !=
		"RABBIT-VRF-RANDOMNESS-V1" {
		t.Fatalf(
			"RandomnessDomainV1 changed: %q",
			RandomnessDomainV1,
		)
	}

	if len(HashToG2DSTV1) != 44 {
		t.Fatalf(
			"unexpected HashToG2 DST length: %d",
			len(HashToG2DSTV1),
		)
	}

	if len(RandomnessDomainV1) != 24 {
		t.Fatalf(
			"unexpected randomness domain length: %d",
			len(RandomnessDomainV1),
		)
	}
}

func TestCryptoProfileV1BaseVectors(t *testing.T) {
	vectors := []cryptoVectorV1{
		{
			name: "sk-1-ascii",
			secret: "00000000000000000000000000000000" +
				"00000000000000000000000000000001",
			message: "5241424249542d5652462d544553542d" +
				"564543544f522d31",
			publicKey: "97f1d3a73197d7942695638c4fa9ac0f" +
				"c3688c4f9774b905a14e3a3f171bac58" +
				"6c55e83ff97a1aeffb3af00adb22c6bb",
			signature: "8644d2af9c0ea1954ebaf81f4eb7f4b8" +
				"055661d08d25819c4dff87adb4b85129" +
				"339cc2b4c708c03eef3300c755c678a1" +
				"03bcd186066d9f68595185f83576b27f" +
				"aad61b21ce37d0711a87825082bad41a" +
				"868b6830e23f9486c31a58942c0c5b7f",
			randomness: "2edc66d7e724e9b5313ae6ce7f4a9c6b" +
				"3df19d3c36039a2468948426ab6e09b2",
		},
		{
			name: "sk-42-ascii",
			secret: "00000000000000000000000000000000" +
				"0000000000000000000000000000002a",
			message: "7261626269742d7672662d746872657368" +
				"6f6c642d6f726465722d746573742d7631",
			publicKey: "8ce3b57b791798433fd323753489cac9b" +
				"ca43b98deaafaed91f4cb010730ae1e3" +
				"8b186ccd37a09b8aed62ce23b699c48",
			signature: "aa1dd236779293aa6128e3f7d4e5bf25" +
				"1d1f168c9cb7e0da60d5fc65c6867d6d" +
				"91009b5f91c6e390c53054baf5edf817" +
				"0b0285fe33efb052ea4ae9236a831d9b" +
				"d57ba3970c66e84c69770db4aede3b12" +
				"7e2c6df6ba83a7eb1612c3812189902c",
			randomness: "d985ddc0341b6045edf1b3621ef612747" +
				"591a09df45207152a3991e3e2655a35",
		},
		{
			name: "sk-q-minus-1-binary",
			secret: "73eda753299d7d483339d80809a1d805" +
				"53bda402fffe5bfeffffffff00000000",
			message: "00010203040506070809fafbfcfdfeff80",
			publicKey: "b7f1d3a73197d7942695638c4fa9ac0f" +
				"c3688c4f9774b905a14e3a3f171bac58" +
				"6c55e83ff97a1aeffb3af00adb22c6bb",
			signature: "8d302eb908567402bbc0f9bb619588f7" +
				"fe979fa56782b66d7e480b4687ca234e" +
				"03b23c8514a60cfa7b60c5a9ed1261b9" +
				"056e374d2dc3354beeacae97927fa602" +
				"684795924a560b6ca698667f2571d58d" +
				"0b3fd3c8c66ac0a6a673d358c8c40ac3",
			randomness: "0b29100ff3edc36549585f05bca234c37" +
				"bae0c0707ea590bbe8f55be8f654310",
		},
	}

	for _, vector := range vectors {
		t.Run(vector.name, func(t *testing.T) {
			secretInput := mustDecodeHexVector(
				t,
				vector.secret,
			)

			message := mustDecodeHexVector(
				t,
				vector.message,
			)

			secretKey, err := SecretKeyFromBytes(
				secretInput,
			)
			if err != nil {
				t.Fatal(err)
			}

			canonicalSecret, err := secretKey.Bytes()
			if err != nil {
				t.Fatal(err)
			}

			if got := hex.EncodeToString(
				canonicalSecret[:],
			); got != vector.secret {
				t.Fatalf(
					"secret mismatch\nwant=%s\ngot =%s",
					vector.secret,
					got,
				)
			}

			publicKey, err := secretKey.PublicKey()
			if err != nil {
				t.Fatal(err)
			}

			if got := hex.EncodeToString(
				publicKey[:],
			); got != vector.publicKey {
				t.Fatalf(
					"public key mismatch\nwant=%s\ngot =%s",
					vector.publicKey,
					got,
				)
			}

			signature, err := secretKey.Sign(
				message,
			)
			if err != nil {
				t.Fatal(err)
			}

			if got := hex.EncodeToString(
				signature[:],
			); got != vector.signature {
				t.Fatalf(
					"signature mismatch\nwant=%s\ngot =%s",
					vector.signature,
					got,
				)
			}

			if err := Verify(
				publicKey,
				message,
				signature,
			); err != nil {
				t.Fatalf(
					"vector verification failed: %v",
					err,
				)
			}

			randomness, err := VerifyAndDeriveRandomness(
				publicKey,
				message,
				signature,
			)
			if err != nil {
				t.Fatal(err)
			}

			if got := hex.EncodeToString(
				randomness[:],
			); got != vector.randomness {
				t.Fatalf(
					"randomness mismatch\nwant=%s\ngot =%s",
					vector.randomness,
					got,
				)
			}
		})
	}
}

func TestCryptoProfileV1RejectsFrModulus(t *testing.T) {
	modulus := mustDecodeHexVector(
		t,
		"73eda753299d7d483339d80809a1d805"+
			"53bda402fffe5bfeffffffff00000001",
	)

	if _, err := SecretKeyFromBytes(
		modulus,
	); err != ErrInvalidSecretKey {
		t.Fatalf(
			"Fr modulus accepted: %v",
			err,
		)
	}
}

func TestCryptoProfileV1ThresholdVector(t *testing.T) {
	message := mustDecodeHexVector(
		t,
		"7261626269742d7672662d746872657368"+
			"6f6c642d6f726465722d746573742d7631",
	)

	masterSecret := mustDecodeHexVector(
		t,
		"00000000000000000000000000000000"+
			"0000000000000000000000000000002a",
	)

	master, err := SecretKeyFromBytes(
		masterSecret,
	)
	if err != nil {
		t.Fatal(err)
	}

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	expectedMasterPublic :=
		"8ce3b57b791798433fd323753489cac9b" +
			"ca43b98deaafaed91f4cb010730ae1e3" +
			"8b186ccd37a09b8aed62ce23b699c48"

	if got := hex.EncodeToString(
		masterPublic[:],
	); got != expectedMasterPublic {
		t.Fatalf(
			"master public key mismatch\nwant=%s\ngot =%s",
			expectedMasterPublic,
			got,
		)
	}

	type thresholdShareVector struct {
		id               uint64
		secret           string
		verificationKey  string
		partialSignature string
	}

	shares := []thresholdShareVector{
		{
			id: 1,
			secret: "00000000000000000000000000000000" +
				"0000000000000000000000000000003c",
			verificationKey: "b783a70a1cf9f53e7d2ddf386bea81a9" +
				"47e5360c5f1e0bf004fceedb2073e4dd" +
				"180ef3d2d91bee7b1c5a88d1afd11c49",
			partialSignature: "b756ad58635ed9522390a916a6080890" +
				"c0a9b7979d5c98978b19129f57a84363" +
				"98f6e6e82410e1d2339f73300e38d024" +
				"11d4359d529383c3fd2c45ba1fd47406" +
				"530a1851bda8f63037c494abb742760b" +
				"24347181ee5e1b76637ae4f954417054",
		},
		{
			id: 3,
			secret: "00000000000000000000000000000000" +
				"000000000000000000000000000000a2",
			verificationKey: "93b15273200e99dbbf91b24f87daa907" +
				"9a023ccdf4debf84d2f9d0c2a1bf57d3" +
				"b13591b62b1c513ec08ad20feb011875",
			partialSignature: "a3decedd4618b01379c8989249b811f9" +
				"7a9b4038f386dc4c4a80641a19b38ec6" +
				"02d35020d3523d7bb8e1bdf7982fd8a7" +
				"09a80ef525ffa9d1259027928d89f795" +
				"a5e064d158d84b0f5e28566550763140" +
				"b929c505cfc03215938cda7058ee123f",
		},
		{
			id: 5,
			secret: "00000000000000000000000000000000" +
				"00000000000000000000000000000160",
			verificationKey: "ac3093600c7c45716cb9baba36022b1c" +
				"0f93714196f91ea6054fd1d0361e981d" +
				"041368afa44d9e8ad41a83d3b710284e",
			partialSignature: "84cd55bda23a4e26cc08313f67273751" +
				"55f56f3dbe12d49073019dcb563c2512" +
				"42a6c449630a20e04626bdf4ae8eea1b" +
				"05be6f81b0bf492493843e640d8107d0" +
				"dc6352e374e3e5190fa40d33a52e30d" +
				"50451ae68d2f064d84344c148bfea7956",
		},
	}

	verified := make(
		[]VerifiedPartialSignature,
		0,
		len(shares),
	)

	for _, vector := range shares {
		scalar := mustFrVector(
			t,
			vector.secret,
		)

		share, err := newSecretShare(
			vector.id,
			scalar,
		)
		if err != nil {
			t.Fatal(err)
		}

		verificationShare, err :=
			share.VerificationShare()
		if err != nil {
			t.Fatal(err)
		}

		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}

		if got := hex.EncodeToString(
			publicKey[:],
		); got != vector.verificationKey {
			t.Fatalf(
				"share %d verification key mismatch\nwant=%s\ngot =%s",
				vector.id,
				vector.verificationKey,
				got,
			)
		}

		partial, err := share.SignPartial(
			message,
		)
		if err != nil {
			t.Fatal(err)
		}

		if got := hex.EncodeToString(
			partial.Signature[:],
		); got != vector.partialSignature {
			t.Fatalf(
				"share %d partial mismatch\nwant=%s\ngot =%s",
				vector.id,
				vector.partialSignature,
				got,
			)
		}

		checked, err := VerifyPartial(
			verificationShare,
			message,
			partial,
		)
		if err != nil {
			t.Fatal(err)
		}

		verified = append(
			verified,
			checked,
		)
	}

	combined, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			verified[2],
			verified[0],
			verified[1],
		},
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	expectedSignature :=
		"aa1dd236779293aa6128e3f7d4e5bf25" +
			"1d1f168c9cb7e0da60d5fc65c6867d6d" +
			"91009b5f91c6e390c53054baf5edf817" +
			"0b0285fe33efb052ea4ae9236a831d9b" +
			"d57ba3970c66e84c69770db4aede3b12" +
			"7e2c6df6ba83a7eb1612c3812189902c"

	if got := hex.EncodeToString(
		combined[:],
	); got != expectedSignature {
		t.Fatalf(
			"threshold signature mismatch\nwant=%s\ngot =%s",
			expectedSignature,
			got,
		)
	}

	direct, err := master.Sign(message)
	if err != nil {
		t.Fatal(err)
	}

	if combined != direct {
		t.Fatal(
			"threshold vector differs from direct master signature",
		)
	}

	randomness, err := VerifyAndDeriveRandomness(
		masterPublic,
		message,
		combined,
	)
	if err != nil {
		t.Fatal(err)
	}

	expectedRandomness :=
		"d985ddc0341b6045edf1b3621ef61274" +
			"7591a09df45207152a3991e3e2655a35"

	if got := hex.EncodeToString(
		randomness[:],
	); got != expectedRandomness {
		t.Fatalf(
			"threshold randomness mismatch\nwant=%s\ngot =%s",
			expectedRandomness,
			got,
		)
	}
}

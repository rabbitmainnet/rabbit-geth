package rabbitvrfstate

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	testScryptN = 2
	testScryptP = 1
)

func dkgTransportKeyStoreFixtureV1(
	t *testing.T,
) (
	lqc.RabbitVRFDKGSessionContextV1,
	lqc.RabbitVRFCommitteeMemberV1,
) {
	t.Helper()

	context, err :=
		lqc.NewRabbitVRFDKGSessionContextV1(
			big.NewInt(9280),
			11,
			crypto.Keccak256Hash(
				[]byte(
					"rabbit-vrf-dkg-store-committee-v1",
				),
			),
			32,
		)
	if err != nil {
		t.Fatal(err)
	}

	participantKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000001",
		)
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-store-ticket-v1",
			),
		),
		Participant: crypto.PubkeyToAddress(
			participantKey.PublicKey,
		),
	}

	return context, member
}

func newTestDKGTransportKeyStoreV1(
	t *testing.T,
	dir string,
) *DKGTransportKeyStoreV1 {
	t.Helper()

	store, err :=
		NewDKGTransportKeyStoreV1(
			dir,
			testScryptN,
			testScryptP,
		)
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func TestDKGTransportKeyStoreV1RestartRecovery(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	dir :=
		filepath.Join(
			t.TempDir(),
			"rabbit-vrf",
			"dkg-transport",
		)

	firstStore :=
		newTestDKGTransportKeyStoreV1(
			t,
			dir,
		)

	privateKey, binding, err :=
		firstStore.Create(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	path, err :=
		firstStore.Path(
			context,
			member,
		)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err :=
		os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	rawPrivateKey :=
		crypto.FromECDSA(
			privateKey,
		)

	if bytes.Contains(
		encoded,
		rawPrivateKey,
	) {
		t.Fatal(
			"raw transport private key found in persisted file",
		)
	}

	rawHex :=
		[]byte(
			hex.EncodeToString(
				rawPrivateKey,
			),
		)

	if bytes.Contains(
		encoded,
		rawHex,
	) {
		t.Fatal(
			"hex transport private key found in persisted file",
		)
	}

	secondStore :=
		newTestDKGTransportKeyStoreV1(
			t,
			dir,
		)

	recovered, recoveredBinding, err :=
		secondStore.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	if privateKey.D.Cmp(
		recovered.D,
	) != 0 {
		t.Fatal(
			"restart recovered a different transport private key",
		)
	}

	if binding != recoveredBinding {
		t.Fatal(
			"restart recovered a different transport binding",
		)
	}

	originalPublic :=
		crypto.CompressPubkey(
			&privateKey.PublicKey,
		)

	recoveredPublic :=
		crypto.CompressPubkey(
			&recovered.PublicKey,
		)

	if !bytes.Equal(
		originalPublic,
		recoveredPublic,
	) {
		t.Fatal(
			"restart changed transport public key",
		)
	}

	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}

		if fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf(
				"transport key file permissions=%04o want=0600",
				fileInfo.Mode().Perm(),
			)
		}

		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}

		if dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf(
				"transport key directory permissions=%04o want=0700",
				dirInfo.Mode().Perm(),
			)
		}
	}
}

func TestDKGTransportKeyStoreV1RejectsWrongPassword(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	_, _, err :=
		store.Create(
			context,
			member,
			"correct-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err :=
		store.Load(
			context,
			member,
			"wrong-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreDecryptV1,
	) {
		t.Fatalf(
			"wrong-password error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1RejectsCiphertextCorruption(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	_, _, err :=
		store.Create(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	path, err :=
		store.Path(
			context,
			member,
		)
	if err != nil {
		t.Fatal(err)
	}

	record :=
		readTestStoreRecordV1(
			t,
			path,
		)

	if len(record.Crypto.CipherText) == 0 {
		t.Fatal(
			"empty encrypted ciphertext",
		)
	}

	ciphertext :=
		[]byte(
			record.Crypto.CipherText,
		)

	if ciphertext[0] == '0' {
		ciphertext[0] = '1'
	} else {
		ciphertext[0] = '0'
	}

	record.Crypto.CipherText =
		string(ciphertext)

	writeTestStoreRecordV1(
		t,
		path,
		record,
	)

	if _, _, err :=
		store.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreDecryptV1,
	) {
		t.Fatalf(
			"corrupt-ciphertext error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1RejectsMetadataTampering(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	_, _, err :=
		store.Create(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	path, err :=
		store.Path(
			context,
			member,
		)
	if err != nil {
		t.Fatal(err)
	}

	record :=
		readTestStoreRecordV1(
			t,
			path,
		)

	record.ShareID++

	writeTestStoreRecordV1(
		t,
		path,
		record,
	)

	if _, _, err :=
		store.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreMetadataMismatchV1,
	) {
		t.Fatalf(
			"metadata-tamper error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1AuthenticatesPublicKeyMetadata(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	_, _, err :=
		store.Create(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	path, err :=
		store.Path(
			context,
			member,
		)
	if err != nil {
		t.Fatal(err)
	}

	record :=
		readTestStoreRecordV1(
			t,
			path,
		)

	otherKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000003",
		)
	if err != nil {
		t.Fatal(err)
	}

	otherPublic, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&otherKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	record.PublicKey =
		otherPublic

	writeTestStoreRecordV1(
		t,
		path,
		record,
	)

	if _, _, err :=
		store.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreMetadataMismatchV1,
	) {
		t.Fatalf(
			"public-key metadata tamper error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1NeverOverwrites(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	firstKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000002",
		)
	if err != nil {
		t.Fatal(err)
	}

	secondKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000003",
		)
	if err != nil {
		t.Fatal(err)
	}

	_, err =
		store.Save(
			context,
			member,
			firstKey,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	path, err :=
		store.Path(
			context,
			member,
		)
	if err != nil {
		t.Fatal(err)
	}

	before, err :=
		os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err :=
		store.Save(
			context,
			member,
			secondKey,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreAlreadyExistsV1,
	) {
		t.Fatalf(
			"overwrite error=%v",
			err,
		)
	}

	after, err :=
		os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(
		before,
		after,
	) {
		t.Fatal(
			"existing transport key store was modified",
		)
	}

	recovered, _, err :=
		store.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	if recovered.D.Cmp(
		firstKey.D,
	) != 0 {
		t.Fatal(
			"overwrite attempt replaced original transport key",
		)
	}
}

func TestDKGTransportKeyStoreV1RejectsMissingState(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	if _, _, err :=
		store.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreMissingV1,
	) {
		t.Fatalf(
			"missing-state error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1RejectsParticipantKeyReuse(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	participantKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000001",
		)
	if err != nil {
		t.Fatal(err)
	}

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	if _, err :=
		store.Save(
			context,
			member,
			participantKey,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		lqc.ErrRabbitVRFDKGTransportKeyReusesParticipantKeyV1,
	) {
		t.Fatalf(
			"participant-key reuse error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1BindsSession(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	_, _, err :=
		store.Create(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	otherContext, err :=
		lqc.NewRabbitVRFDKGSessionContextV1(
			big.NewInt(9280),
			12,
			crypto.Keccak256Hash(
				[]byte(
					"rabbit-vrf-dkg-store-committee-v1",
				),
			),
			32,
		)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err :=
		store.Load(
			otherContext,
			member,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreMissingV1,
	) {
		t.Fatalf(
			"cross-session load error=%v",
			err,
		)
	}
}

func readTestStoreRecordV1(
	t *testing.T,
	path string,
) dkgTransportKeyStoreFileV1 {
	t.Helper()

	encoded, err :=
		os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var record dkgTransportKeyStoreFileV1

	if err :=
		json.Unmarshal(
			encoded,
			&record,
		); err != nil {
		t.Fatal(err)
	}

	return record
}

func writeTestStoreRecordV1(
	t *testing.T,
	path string,
	record dkgTransportKeyStoreFileV1,
) {
	t.Helper()

	encoded, err :=
		json.MarshalIndent(
			record,
			"",
			"  ",
		)
	if err != nil {
		t.Fatal(err)
	}

	encoded =
		append(
			encoded,
			'\n',
		)

	if err :=
		os.WriteFile(
			path,
			encoded,
			0o600,
		); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		if err :=
			os.Chmod(
				path,
				0o600,
			); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDKGTransportKeyStoreV1RejectsZeroSessionMetadata(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	_, _, err :=
		store.Create(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	path, err :=
		store.Path(
			context,
			member,
		)
	if err != nil {
		t.Fatal(err)
	}

	record :=
		readTestStoreRecordV1(
			t,
			path,
		)

	record.SessionID =
		common.Hash{}

	writeTestStoreRecordV1(
		t,
		path,
		record,
	)

	if _, _, err :=
		store.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrDKGTransportKeyStoreMetadataMismatchV1,
	) {
		t.Fatalf(
			"zero-session metadata error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1NilCreateFailsClosed(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	var store *DKGTransportKeyStoreV1

	if _, _, err :=
		store.Create(
			context,
			member,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrInvalidDKGTransportKeyStoreV1,
	) {
		t.Fatalf(
			"nil-store create error=%v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1RejectsInconsistentPrivateKey(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	store :=
		newTestDKGTransportKeyStoreV1(
			t,
			t.TempDir(),
		)

	firstKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000002",
		)
	if err != nil {
		t.Fatal(err)
	}

	secondKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000003",
		)
	if err != nil {
		t.Fatal(err)
	}

	forged :=
		*firstKey

	forged.PublicKey =
		secondKey.PublicKey

	if _, err :=
		store.Save(
			context,
			member,
			&forged,
			"rabbit-vrf-store-password",
		); !errors.Is(
		err,
		ErrInvalidDKGTransportKeyStoreV1,
	) {
		t.Fatalf(
			"inconsistent-private-key error=%v",
			err,
		)
	}

	path, err :=
		store.Path(
			context,
			member,
		)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf(
			"invalid private key created persistent state: %v",
			err,
		)
	}
}

func TestDKGTransportKeyStoreV1ConcurrentSaveNeverOverwrites(
	t *testing.T,
) {
	context, member :=
		dkgTransportKeyStoreFixtureV1(t)

	dir :=
		filepath.Join(
			t.TempDir(),
			"rabbit-vrf",
			"dkg-transport",
		)

	firstStore :=
		newTestDKGTransportKeyStoreV1(
			t,
			dir,
		)

	secondStore :=
		newTestDKGTransportKeyStoreV1(
			t,
			dir,
		)

	firstKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000002",
		)
	if err != nil {
		t.Fatal(err)
	}

	secondKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000003",
		)
	if err != nil {
		t.Fatal(err)
	}

	start :=
		make(
			chan struct{},
		)

	results :=
		make(
			chan error,
			2,
		)

	go func() {
		<-start

		_, err :=
			firstStore.Save(
				context,
				member,
				firstKey,
				"rabbit-vrf-store-password",
			)

		results <- err
	}()

	go func() {
		<-start

		_, err :=
			secondStore.Save(
				context,
				member,
				secondKey,
				"rabbit-vrf-store-password",
			)

		results <- err
	}()

	close(start)

	firstErr := <-results
	secondErr := <-results

	successCount := 0
	existsCount := 0

	for _, err := range []error{
		firstErr,
		secondErr,
	} {
		switch {
		case err == nil:
			successCount++

		case errors.Is(
			err,
			ErrDKGTransportKeyStoreAlreadyExistsV1,
		):
			existsCount++

		default:
			t.Fatalf(
				"unexpected concurrent save error=%v",
				err,
			)
		}
	}

	if successCount != 1 ||
		existsCount != 1 {
		t.Fatalf(
			"concurrent save results: success=%d exists=%d",
			successCount,
			existsCount,
		)
	}

	recovered, _, err :=
		firstStore.Load(
			context,
			member,
			"rabbit-vrf-store-password",
		)
	if err != nil {
		t.Fatal(err)
	}

	if recovered.D.Cmp(
		firstKey.D,
	) != 0 &&
		recovered.D.Cmp(
			secondKey.D,
		) != 0 {
		t.Fatal(
			"concurrent persistence recovered unexpected key",
		)
	}
}

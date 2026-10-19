//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"bytes"
	"crypto/ecdsa"
	"errors"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

var errRabbitVRFDKGTransportReusesP2PKeyV1 = errors.New(
	"rabbit vrf dkg transport key reuses p2p node key v1",
)

type rabbitVRFDKGTransportKeyGeneratorV1 func() (*ecdsa.PrivateKey, error)

func rabbitVRFDKGValidateAndZeroTransportKeyV1(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	privateKey *ecdsa.PrivateKey,
	binding lqc.RabbitVRFDKGTransportKeyBindingV1,
	p2pKey *ecdsa.PrivateKey,
) error {
	if privateKey == nil {
		return errRabbitVRFDKGRuntimeV1
	}
	defer zeroRabbitVRFDKGPrivateKeyV1(privateKey)

	if privateKey.D == nil ||
		privateKey.PublicKey.X == nil ||
		privateKey.PublicKey.Y == nil ||
		p2pKey == nil ||
		p2pKey.PublicKey.X == nil ||
		p2pKey.PublicKey.Y == nil {
		return errRabbitVRFDKGRuntimeV1
	}

	if _, err := lqc.VerifyRabbitVRFDKGTransportKeyBindingV1(
		context,
		member,
		binding,
	); err != nil {
		return err
	}

	transportPublic, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(&privateKey.PublicKey),
		)
	if err != nil || transportPublic != binding.PublicKey {
		return errRabbitVRFDKGRuntimeV1
	}

	if bytes.Equal(
		transportPublic[:],
		crypto.CompressPubkey(&p2pKey.PublicKey),
	) {
		return errRabbitVRFDKGTransportReusesP2PKeyV1
	}

	return nil
}

func rabbitVRFDKGLoadOrCreateTransportBindingV1(
	store *rabbitvrfstate.DKGTransportKeyStoreV1,
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	password string,
	p2pKey *ecdsa.PrivateKey,
) (lqc.RabbitVRFDKGTransportKeyBindingV1, error) {
	return rabbitVRFDKGLoadOrCreateTransportBindingWithGeneratorV1(
		store,
		context,
		member,
		password,
		p2pKey,
		crypto.GenerateKey,
	)
}

func rabbitVRFDKGLoadOrCreateTransportBindingWithGeneratorV1(
	store *rabbitvrfstate.DKGTransportKeyStoreV1,
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	password string,
	p2pKey *ecdsa.PrivateKey,
	generate rabbitVRFDKGTransportKeyGeneratorV1,
) (lqc.RabbitVRFDKGTransportKeyBindingV1, error) {
	var empty lqc.RabbitVRFDKGTransportKeyBindingV1

	if store == nil ||
		password == "" ||
		p2pKey == nil ||
		p2pKey.PublicKey.X == nil ||
		p2pKey.PublicKey.Y == nil ||
		generate == nil {
		return empty, errRabbitVRFDKGRuntimeV1
	}

	privateKey, binding, err :=
		store.Load(context, member, password)
	if err == nil {
		if err := rabbitVRFDKGValidateAndZeroTransportKeyV1(
			context,
			member,
			privateKey,
			binding,
			p2pKey,
		); err != nil {
			return empty, err
		}
		return binding, nil
	}
	if privateKey != nil {
		zeroRabbitVRFDKGPrivateKeyV1(privateKey)
	}
	if !errors.Is(
		err,
		rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1,
	) {
		return empty, err
	}

	privateKey, err = generate()
	if err != nil {
		return empty, err
	}
	if privateKey == nil ||
		privateKey.D == nil ||
		privateKey.PublicKey.X == nil ||
		privateKey.PublicKey.Y == nil {
		zeroRabbitVRFDKGPrivateKeyV1(privateKey)
		return empty, errRabbitVRFDKGRuntimeV1
	}

	transportPublic :=
		crypto.CompressPubkey(&privateKey.PublicKey)
	if bytes.Equal(
		transportPublic,
		crypto.CompressPubkey(&p2pKey.PublicKey),
	) {
		zeroRabbitVRFDKGPrivateKeyV1(privateKey)
		return empty, errRabbitVRFDKGTransportReusesP2PKeyV1
	}

	binding, err =
		store.Save(
			context,
			member,
			privateKey,
			password,
		)
	if err == nil {
		if err := rabbitVRFDKGValidateAndZeroTransportKeyV1(
			context,
			member,
			privateKey,
			binding,
			p2pKey,
		); err != nil {
			return empty, err
		}
		return binding, nil
	}

	zeroRabbitVRFDKGPrivateKeyV1(privateKey)

	if !errors.Is(
		err,
		rabbitvrfstate.ErrDKGTransportKeyStoreAlreadyExistsV1,
	) {
		return empty, err
	}

	privateKey, binding, err =
		store.Load(context, member, password)
	if err != nil {
		if privateKey != nil {
			zeroRabbitVRFDKGPrivateKeyV1(privateKey)
		}
		return empty, err
	}

	if err := rabbitVRFDKGValidateAndZeroTransportKeyV1(
		context,
		member,
		privateKey,
		binding,
		p2pKey,
	); err != nil {
		return empty, err
	}

	return binding, nil
}

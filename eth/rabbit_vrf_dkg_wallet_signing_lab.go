//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func rabbitVRFDKGSignTransportBindingEnvelopeV1(
	wallets []accounts.Wallet,
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	binding lqc.RabbitVRFDKGTransportKeyBindingV1,
) (
	lqc.RabbitVRFDKGEnvelopeV1,
	error,
) {
	var empty lqc.RabbitVRFDKGEnvelopeV1

	root, err := lqc.VerifyRabbitVRFDKGTransportKeyBindingV1(
		context,
		member,
		binding,
	)
	if err != nil {
		return empty, fmt.Errorf(
			"verify rabbit vrf dkg transport binding: %w",
			err,
		)
	}

	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
		context,
		member,
		lqc.RabbitVRFDKGMessageTransportKeyBindingV1,
		root,
	)
	if err != nil {
		return empty, fmt.Errorf(
			"create rabbit vrf dkg transport envelope: %w",
			err,
		)
	}

	signingData, err := lqc.RabbitVRFDKGEnvelopeSigningDataV1(
		context,
		envelope,
	)
	if err != nil {
		return empty, fmt.Errorf(
			"encode rabbit vrf dkg transport envelope signing data: %w",
			err,
		)
	}

	var lastErr error

	for _, wallet := range wallets {
		for _, account := range wallet.Accounts() {
			if account.Address != member.Participant {
				continue
			}

			signature, signErr := wallet.SignData(
				account,
				accounts.MimetypeClique,
				signingData,
			)
			if signErr != nil {
				lastErr = signErr
				continue
			}

			envelope.Signature = append(
				[]byte(nil),
				signature...,
			)

			if verifyErr := lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
				context,
				member,
				binding,
				envelope,
			); verifyErr != nil {
				lastErr = verifyErr
				envelope.Signature = nil
				continue
			}

			return envelope, nil
		}
	}

	if lastErr != nil {
		return empty, fmt.Errorf(
			"sign rabbit vrf dkg transport envelope: %w",
			lastErr,
		)
	}

	return empty, fmt.Errorf(
		"%w: participant wallet unavailable",
		errRabbitVRFDKGRuntimeV1,
	)
}

package rabbitvrf

import (
	"errors"
	"testing"
)

func TestAggregateDistinctMessagesV1CanonicalAndAdversarial(t *testing.T) {
	shares, _ := testThresholdShares(t, 5)

	verificationShares := make([]VerificationShare, 3)
	messages := [][]byte{
		[]byte("rabbit-vrf-participation-share-1"),
		[]byte("rabbit-vrf-participation-share-2"),
		[]byte("rabbit-vrf-participation-share-3"),
	}
	signatures := make([]Signature, 3)

	for index := range verificationShares {
		verificationShare, err := shares[index].VerificationShare()
		if err != nil {
			t.Fatal(err)
		}
		verificationShares[index] = verificationShare

		partial, err := shares[index].SignPartial(messages[index])
		if err != nil {
			t.Fatal(err)
		}
		signatures[index] = partial.Signature
	}

	aggregate, err := AggregateSignaturesV1(signatures)
	if err != nil {
		t.Fatalf("aggregate valid signatures: %v", err)
	}
	if err := VerifyAggregateDistinctMessagesV1(
		verificationShares,
		messages,
		aggregate,
	); err != nil {
		t.Fatalf("verify valid aggregate: %v", err)
	}

	reorderedShares := []VerificationShare{
		verificationShares[2],
		verificationShares[0],
		verificationShares[1],
	}
	reorderedMessages := [][]byte{
		messages[2],
		messages[0],
		messages[1],
	}
	if err := VerifyAggregateDistinctMessagesV1(
		reorderedShares,
		reorderedMessages,
		aggregate,
	); err != nil {
		t.Fatalf("pair-preserving reorder rejected: %v", err)
	}

	t.Run("wrong_message", func(t *testing.T) {
		bad := append([][]byte(nil), messages...)
		bad[1] = []byte("rabbit-vrf-participation-share-2-mutated")
		if err := VerifyAggregateDistinctMessagesV1(verificationShares, bad, aggregate); err == nil {
			t.Fatal("mutated participant message accepted")
		}
	})

	t.Run("swapped_messages", func(t *testing.T) {
		bad := append([][]byte(nil), messages...)
		bad[0], bad[1] = bad[1], bad[0]
		if err := VerifyAggregateDistinctMessagesV1(verificationShares, bad, aggregate); err == nil {
			t.Fatal("share/message relabel accepted")
		}
	})

	t.Run("bad_aggregate_signature", func(t *testing.T) {
		bad := aggregate
		bad[len(bad)-1] ^= 0x01
		if err := VerifyAggregateDistinctMessagesV1(verificationShares, messages, bad); err == nil {
			t.Fatal("mutated aggregate signature accepted")
		}
	})

	t.Run("duplicate_share", func(t *testing.T) {
		badShares := append([]VerificationShare(nil), verificationShares...)
		badShares[1] = badShares[0]
		if err := VerifyAggregateDistinctMessagesV1(badShares, messages, aggregate); !errors.Is(err, ErrDuplicateAggregateShareV1) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("duplicate_message", func(t *testing.T) {
		badMessages := append([][]byte(nil), messages...)
		badMessages[1] = badMessages[0]
		if err := VerifyAggregateDistinctMessagesV1(verificationShares, badMessages, aggregate); !errors.Is(err, ErrDuplicateAggregateMessageV1) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("length_mismatch", func(t *testing.T) {
		if err := VerifyAggregateDistinctMessagesV1(verificationShares[:2], messages, aggregate); !errors.Is(err, ErrInvalidAggregateSignatureV1) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if _, err := AggregateSignaturesV1(nil); !errors.Is(err, ErrInvalidAggregateSignatureV1) {
			t.Fatalf("aggregate error=%v", err)
		}
		if err := VerifyAggregateDistinctMessagesV1(nil, nil, Signature{}); !errors.Is(err, ErrInvalidAggregateSignatureV1) {
			t.Fatalf("verify error=%v", err)
		}
	})

	t.Run("same_message_threshold_partials_do_not_authenticate_distinct_messages", func(t *testing.T) {
		commonMessage := []byte("rabbit-vrf-threshold-common-message")
		commonSignatures := make([]Signature, 3)
		for index := range commonSignatures {
			partial, err := shares[index].SignPartial(commonMessage)
			if err != nil {
				t.Fatal(err)
			}
			commonSignatures[index] = partial.Signature
		}
		commonAggregate, err := AggregateSignaturesV1(commonSignatures)
		if err != nil {
			t.Fatal(err)
		}
		if err := VerifyAggregateDistinctMessagesV1(verificationShares, messages, commonAggregate); err == nil {
			t.Fatal("same-message threshold partials authenticated distinct participation messages")
		}
	})
}

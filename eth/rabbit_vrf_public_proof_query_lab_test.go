//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	vrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
)

type rabbitVRFPublicProofChainFixtureV1 struct {
	head    *types.Header
	request *types.Header
	genesis *types.Block
	config  *params.ChainConfig
}

func (c *rabbitVRFPublicProofChainFixtureV1) CurrentBlock() *types.Header { return c.head }
func (c *rabbitVRFPublicProofChainFixtureV1) Genesis() *types.Block       { return c.genesis }
func (c *rabbitVRFPublicProofChainFixtureV1) Config() *params.ChainConfig { return c.config }
func (c *rabbitVRFPublicProofChainFixtureV1) GetHeaderByNumber(n uint64) *types.Header {
	if c.request != nil && c.request.Number.Uint64() == n {
		return c.request
	}
	if c.head != nil && c.head.Number.Uint64() == n {
		return c.head
	}
	return nil
}

func rabbitVRFPublicProofQueryFixtureV1(t *testing.T) (rabbitVRFPublicProofQueryV1, *rabbitVRFPublicProofChainFixtureV1, common.Hash) {
	t.Helper()
	session, members, _, certificate := rabbitVRFAdversarialCertificateV1(t)
	sid, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}
	id := common.HexToHash("0x334455")
	head := &types.Header{Number: big.NewInt(1281), Time: 12810, Extra: []byte("request-anchor")}
	chain := &rabbitVRFPublicProofChainFixtureV1{
		head: head, request: types.CopyHeader(head),
		genesis: types.NewBlockWithHeader(&types.Header{Number: big.NewInt(0)}),
		config:  &params.ChainConfig{ChainID: new(big.Int).Set(session.ChainID), LQC: &params.LQCConfig{EpochLength: 128, TargetBlockTimeMs: 10000}},
	}
	message, messageHash, err := lqc.RabbitVRFThresholdMessageV1(session, certificate.KeysetRoot, id)
	if err != nil {
		t.Fatal(err)
	}
	keyset := rabbitvrfstate.DKGFinalKeysetV1{
		KeysetRoot: certificate.KeysetRoot, ThresholdPublicKey: certificate.ThresholdPublicKey,
		TranscriptRoot: certificate.TranscriptRoot, VerificationShares: certificate.VerificationShareSamples,
	}
	var partials []vrf.PartialSignature
	var participations []vrf.Signature
	var ids []uint64
	for i, scalar := range []uint64{10, 13, 16} {
		share := rabbitVRFTestSecretShareV1(t, uint64(i+1), scalar)
		defer share.Destroy()
		partial, _, err := rabbitVRFSignThresholdPartialWithKeysetV1(share, uint64(i+1), keyset, message)
		if err != nil {
			t.Fatal(err)
		}
		partials = append(partials, partial)
		participation, _, err := lqc.RabbitVRFCompactParticipationMessageV1(session, certificate.KeysetRoot, id, messageHash, members[i])
		if err != nil {
			t.Fatal(err)
		}
		signed, err := share.SignPartial(participation)
		if err != nil {
			t.Fatal(err)
		}
		participations = append(participations, signed.Signature)
		ids = append(ids, uint64(i+1))
	}
	signature, randomness, err := rabbitVRFCombineThresholdPartialsWithKeysetV1(session.Threshold, keyset, message, partials)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := vrf.AggregateSignaturesV1(participations)
	if err != nil {
		t.Fatal(err)
	}
	bitmap, err := lqc.RabbitVRFCompactParticipationFixedBitmapV1(session, ids)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := lqc.RabbitVRFFinalizationProofHashWithParticipationV1(id, session.TargetVRFEpoch, 1281, signature[:], bitmap, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	proof := lqc.RabbitVRFFinalizationV1{Version: lqc.RabbitVRFFinalizationVersionV1, RequestID: id, KeysetRoot: certificate.KeysetRoot,
		Epoch: session.TargetVRFEpoch, Round: 1281, Randomness: randomness, ProofHash: hash, Signature: signature,
		ParticipationBitmap: bitmap, ParticipationAggregateSignature: aggregate}
	query := rabbitVRFPublicProofQueryV1{
		chain: chain, syncing: func() bool { return false },
		request: func(requestID common.Hash) (rabbitVRFCanonicalRequestV1, error) {
			return rabbitVRFCanonicalRequestV1{RequestID: requestID, Requester: members[0].Participant, RequestBlock: 1281, Status: rabbitVRFRequestStatusPendingV1}, nil
		},
		bridge: func(uint64) (lqc.RabbitVRFDKGBridgeV1, uint64, bool, error) {
			return lqc.RabbitVRFDKGBridgeV1{Session: session, SessionID: sid, Members: members, TargetVRFEpoch: session.TargetVRFEpoch}, 128, true, nil
		},
		certificate: func(lqc.RabbitVRFDKGSessionContextV1, []lqc.RabbitVRFCommitteeMemberV1) (lqc.RabbitVRFKeysetCertificateV1, error) {
			return certificate, nil
		},
		proof: func(common.Hash, common.Hash) (lqc.RabbitVRFFinalizationV1, bool) { return proof, true },
	}
	return query, chain, id
}

func TestRabbitVRFPublicProofQueryVerifiedBeforeSettlementV1(t *testing.T) {
	q, chain, id := rabbitVRFPublicProofQueryFixtureV1(t)
	result, err := q.read(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "proof_ready_unsettled" || !result.CanonicalAtHead || result.OnChainConfirmed || result.ConsensusFinalized {
		t.Fatalf("incorrect settlement/finality status: %+v", result)
	}
	if result.HeadHash != chain.head.Hash() || result.RequestBlockHash != chain.request.Hash() {
		t.Fatal("missing canonical anchor")
	}
	var session lqc.RabbitVRFDKGSessionContextV1
	var members []lqc.RabbitVRFCommitteeMemberV1
	var certificate lqc.RabbitVRFKeysetCertificateV1
	var proof lqc.RabbitVRFFinalizationV1
	for _, v := range []struct {
		value []byte
		out   interface{}
	}{
		{result.SessionRLP, &session}, {result.MembersRLP, &members}, {result.CertificateRLP, &certificate}, {result.FinalizationRLP, &proof},
	} {
		if err := rlp.DecodeBytes(v.value, v.out); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lqc.ValidateRabbitVRFKeysetCertificateV1(session, members, certificate); err != nil {
		t.Fatal(err)
	}
	if err := lqc.ValidateRabbitVRFFinalizationProofV1(session, members, certificate, proof); err != nil {
		t.Fatal(err)
	}
	if proof.Randomness != result.Randomness || proof.ProofHash != result.ProofHash || proof.RequestID != id {
		t.Fatal("RPC result differs from verifiable proof")
	}
}

func TestRabbitVRFPublicProofQueryRejectsChangedContextV1(t *testing.T) {
	for _, fault := range []string{"signature", "certificate", "round", "session", "request", "reorg", "advance", "sync", "already-settled", "missing-header"} {
		t.Run(fault, func(t *testing.T) {
			q, chain, id := rabbitVRFPublicProofQueryFixtureV1(t)
			proofReader, certReader, bridgeReader, requestReader := q.proof, q.certificate, q.bridge, q.request
			switch fault {
			case "signature", "round":
				q.proof = func(s, r common.Hash) (lqc.RabbitVRFFinalizationV1, bool) {
					p, ok := proofReader(s, r)
					if fault == "signature" {
						p.Signature[0] ^= 1
					} else {
						p.Round++
					}
					return p, ok
				}
			case "certificate":
				q.certificate = func(s lqc.RabbitVRFDKGSessionContextV1, m []lqc.RabbitVRFCommitteeMemberV1) (lqc.RabbitVRFKeysetCertificateV1, error) {
					c, e := certReader(s, m)
					c.Signatures[0][0] ^= 1
					return c, e
				}
			case "session":
				q.bridge = func(n uint64) (lqc.RabbitVRFDKGBridgeV1, uint64, bool, error) {
					b, l, ok, e := bridgeReader(n)
					b.SessionID[0] ^= 1
					return b, l, ok, e
				}
			case "request", "already-settled":
				q.request = func(r common.Hash) (rabbitVRFCanonicalRequestV1, error) {
					v, e := requestReader(r)
					if fault == "request" {
						v.RequestID[0] ^= 1
					} else {
						v.Status = 2
					}
					return v, e
				}
			case "missing-header":
				chain.request = nil
				chain.head = types.CopyHeader(chain.head)
				chain.head.Number.Add(chain.head.Number, big.NewInt(1))
			case "reorg", "advance", "sync":
				syncing := false
				q.syncing = func() bool { return syncing }
				q.certificate = func(s lqc.RabbitVRFDKGSessionContextV1, m []lqc.RabbitVRFCommitteeMemberV1) (lqc.RabbitVRFKeysetCertificateV1, error) {
					c, e := certReader(s, m)
					if fault == "sync" {
						syncing = true
					} else {
						chain.head = types.CopyHeader(chain.head)
						if fault == "advance" {
							chain.head.Number.Add(chain.head.Number, big.NewInt(1))
						} else {
							chain.head.Extra = []byte("different-fork")
						}
					}
					return c, e
				}
			}
			if result, err := q.read(context.Background(), id); err == nil || result != nil {
				t.Fatalf("%s exposed stale/unverified result: %+v %v", fault, result, err)
			}
		})
	}
}

func TestRabbitVRFPublicProofQueryPendingAndCancelledV1(t *testing.T) {
	q, _, id := rabbitVRFPublicProofQueryFixtureV1(t)
	q.proof = func(common.Hash, common.Hash) (lqc.RabbitVRFFinalizationV1, bool) {
		return lqc.RabbitVRFFinalizationV1{}, false
	}
	q.certificate = func(lqc.RabbitVRFDKGSessionContextV1, []lqc.RabbitVRFCommitteeMemberV1) (lqc.RabbitVRFKeysetCertificateV1, error) {
		t.Fatal("pending query loaded certificate")
		return lqc.RabbitVRFKeysetCertificateV1{}, nil
	}
	result, err := q.read(context.Background(), id)
	if err != nil || result.Status != "pending" || len(result.FinalizationRLP) != 0 || result.OnChainConfirmed {
		t.Fatalf("pending was exposed as result: %+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := q.read(ctx, id); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("cancelled query: %+v %v", result, err)
	}
}

func TestRabbitVRFPublicProofQueryRPCAndBoundV1(t *testing.T) {
	server := rpc.NewServer()
	defer server.Stop()
	if err := server.RegisterName("lqc", &LQCRegistryAPI{}); err != nil {
		t.Fatal(err)
	}
	client := rpc.DialInProc(server)
	defer client.Close()
	var out RabbitVRFPublicProofResultV1
	err := client.Call(&out, "lqc_rabbitVRFProof", common.Hash{1})
	if err == nil || !strings.Contains(err.Error(), "runtime unavailable") {
		t.Fatalf("RPC name not registered or unsafe nil backend: %v", err)
	}
	for i := 0; i < cap(rabbitVRFPublicProofSlotsV1); i++ {
		rabbitVRFPublicProofSlotsV1 <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(rabbitVRFPublicProofSlotsV1); i++ {
			<-rabbitVRFPublicProofSlotsV1
		}
	}()
	api := &LQCRegistryAPI{eth: &Ethereum{blockchain: &core.BlockChain{}, vrfDKGTransport: &rabbitVRFDKGTransport{runtime: &rabbitVRFDKGRuntime{}}}}
	if _, err := api.RabbitVRFProof(context.Background(), common.Hash{1}); err == nil || !strings.Contains(err.Error(), "query busy") {
		t.Fatalf("unbounded verification concurrency: %v", err)
	}
}

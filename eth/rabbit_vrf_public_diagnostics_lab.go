//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import "errors"

// VRFDiagnostics exposes public session progress only.
// It never reads or returns private keys, evaluations or secret shares.
func (api *LQCRegistryAPI) VRFDiagnostics() (map[string]interface{}, error) {
	if api == nil || api.eth == nil || api.eth.vrfDKGRuntime == nil {
		return nil, errors.New("VRF runtime unavailable")
	}
	runtime := api.eth.vrfDKGRuntime
	context := runtime.currentContext()
	out := map[string]interface{}{
		"head":             context.HeadNumber,
		"headHash":         context.HeadHash,
		"session":          context.SessionID,
		"sourceEpoch":      context.SourceWorkEpoch,
		"preparationEpoch": context.PreparationEpoch,
		"targetEpoch":      context.TargetVRFEpoch,
		"committeeSize":    context.CanonicalSession.CommitteeSize,
		"threshold":        context.CanonicalSession.Threshold,
		"localMembers":     len(context.Members),
		"runtimeReady":     runtime.secretReadyV1(),
	}
	transport := api.eth.vrfDKGTransport
	if transport == nil {
		out["transportAvailable"] = false
		return out, nil
	}
	out["transportAvailable"] = true
	members := make([]map[string]interface{}, 0, len(context.CanonicalMembers))
	received := 0
	for _, member := range context.CanonicalMembers {
		_, present := transport.commitmentForDealerV1(
			context.SessionID, member.ShareID,
		)
		if present {
			received++
		}
		members = append(members, map[string]interface{}{
			"address":            member.Participant,
			"shareId":            member.ShareID,
			"commitmentReceived": present,
		})
	}
	out["members"] = members
	out["commitmentsReceived"] = received
	root, bindings, _, complete, err := transport.canonicalTransportKeySetV1()
	out["transportKeysetRoot"] = root
	out["transportBindingsReceived"] = len(bindings)
	out["transportKeysetComplete"] = complete
	if err != nil {
		out["transportKeysetError"] = err.Error()
	}
	transport.mu.RLock()
	out["transportPeers"] = len(transport.peers)
	if transport.keysetCertificateSession == context.SessionID {
		out["certificateSignatures"] = len(transport.keysetCertificateSignatures)
	} else {
		out["certificateSignatures"] = 0
	}
	transport.mu.RUnlock()
	if runtime.currentContext().SessionID != context.SessionID {
		return nil, errors.New("VRF session changed; retry diagnostic")
	}
	return out, nil
}

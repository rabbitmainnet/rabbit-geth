//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

// Certificate production belongs to preparation, independently of block inclusion.
func (runtime *rabbitVRFDKGRuntime) progressPreparationCertificateV1() error {
	if runtime == nil || runtime.backend == nil || runtime.backend.vrfDKGTransport == nil {
		return nil
	}
	transport := runtime.backend.vrfDKGTransport
	if err := transport.restorePersistedKeysetCertificateV1(); err != nil {
		return err
	}
	return transport.publishLocalKeysetCertificateSignaturesV1()
}

package lqc

type ProducerMode byte

const (
	ProducerModeUnknown        ProducerMode = 0
	ProducerModeDevnetSingle   ProducerMode = 1
	ProducerModeCommitteeQueue ProducerMode = 2
	ProducerModeFallbackQueue  ProducerMode = 3
)

func (m ProducerMode) Valid() bool {
	switch m {
	case ProducerModeDevnetSingle, ProducerModeCommitteeQueue, ProducerModeFallbackQueue:
		return true
	default:
		return false
	}
}

func (m ProducerMode) String() string {
	switch m {
	case ProducerModeDevnetSingle:
		return "devnet-single"
	case ProducerModeCommitteeQueue:
		return "committee-queue"
	case ProducerModeFallbackQueue:
		return "fallback-queue"
	default:
		return "unknown"
	}
}

package eth

import (
	"os"

	"github.com/ethereum/go-ethereum/log"
)

func init() {
	if os.Getenv("RABBIT_SYNC_DIAGNOSTIC") == "1" {
		log.SetDefault(log.NewLogger(
			log.NewTerminalHandlerWithLevel(os.Stdout, log.LevelTrace, false),
		))
	}
}

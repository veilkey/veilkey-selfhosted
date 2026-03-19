package api

import (
	"log"
	"time"

	"veilkey-vaultcenter/internal/db"
)

func StartTempRefGC(database *db.DB, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			count, err := database.DeleteExpiredTempRefs()
			if err != nil {
				log.Printf("temp ref GC error: %v", err)
			} else if count > 0 {
				log.Printf("temp ref GC: deleted %d expired refs", count)
			}
			regCount, regErr := database.DeleteExpiredRegistrationTokens()
			if regErr != nil {
				log.Printf("registration token GC error: %v", regErr)
			} else if regCount > 0 {
				log.Printf("registration token GC: expired %d tokens", regCount)
			}
		}
	}
}

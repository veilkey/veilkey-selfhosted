package api

import (
	"log"
	"time"

	"veilkey-keycenter/internal/db"
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
				continue
			}
			if count > 0 {
				log.Printf("temp ref GC: deleted %d expired refs", count)
			}
		}
	}
}

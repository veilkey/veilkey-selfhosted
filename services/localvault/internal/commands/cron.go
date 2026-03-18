package commands

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func RunCron() {
	action := "tick"
	if len(os.Args) > 2 {
		action = os.Args[2]
	}
	switch action {
	case "tick":
		server, _, listenPort := mustLoadServer()
		if deleted, err := server.CleanupExpiredTestFunctions(time.Now()); err != nil {
			log.Fatalf("cron tick cleanup failed: %v", err)
		} else if deleted > 0 {
			log.Printf("cron tick deleted %d expired TEST functions", deleted)
		}
		hubURL := server.LogResolvedVaultcenterURL("cron")
		if hubURL == "" {
			log.Fatal("VEILKEY_VAULTCENTER_URL is required for cron tick")
		}
		globalEndpoint := strings.TrimRight(hubURL, "/") + "/api/functions/global"
		if upserted, removed, err := server.SyncGlobalFunctions(globalEndpoint); err != nil {
			log.Fatalf("cron tick global function sync failed: %v", err)
		} else if upserted > 0 || removed > 0 {
			log.Printf("cron tick synced global functions: upserted=%d removed=%d", upserted, removed)
		}
		hostname, _ := os.Hostname()
		endpoint := strings.TrimRight(hubURL, "/") + "/api/agents/heartbeat"
		if err := server.SendHeartbeatOnce(endpoint, hostname, listenPort); err != nil {
			if err.Error() == "rotation_required" {
				if retryErr := server.SendHeartbeatOnce(endpoint, hostname, listenPort); retryErr != nil {
					log.Fatalf("cron tick failed after rotation update: %v", retryErr)
				}
				fmt.Println("rotation applied and heartbeat sent")
				return
			}
			log.Fatalf("cron tick failed: %v", err)
		}
		fmt.Println("heartbeat sent")
	default:
		fmt.Println("Usage: veilkey-localvault cron tick")
		os.Exit(1)
	}
}

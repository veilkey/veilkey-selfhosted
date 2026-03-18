package hkm

import "time"

func (h *Handler) advancePendingRotationsBestEffort() {
	_, _ = h.deps.DB().AdvancePendingRotations(time.Now().UTC())
}

package recall

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultWithinMinutes = 30
	maxWithinMinutes     = 24 * 60
)

// ReceiptCheck reports whether a successful recall happened recently for a session.
type ReceiptCheck struct {
	OK            bool     `json:"ok"`
	NeedRecall    bool     `json:"need_recall"`
	WithinMinutes int      `json:"within_minutes"`
	Latest        *Receipt `json:"latest,omitempty"`
	Message       string   `json:"message"`
}

func normalizeWithinMinutes(n int) int {
	if n <= 0 {
		return defaultWithinMinutes
	}
	if n > maxWithinMinutes {
		return maxWithinMinutes
	}
	return n
}

func evaluateReceiptCheck(list []Receipt, withinMinutes int, now time.Time) ReceiptCheck {
	withinMinutes = normalizeWithinMinutes(withinMinutes)
	out := ReceiptCheck{WithinMinutes: withinMinutes, NeedRecall: true}
	cutoff := now.Add(-time.Duration(withinMinutes) * time.Minute)
	for i := range list {
		r := list[i]
		if r.QueriedAt.Before(cutoff) {
			continue
		}
		cp := r
		out.OK = true
		out.NeedRecall = false
		out.Latest = &cp
		out.Message = fmt.Sprintf("recall ok within %d minutes (hits=%d source=%s)", withinMinutes, r.HitCount, r.Source)
		return out
	}
	out.Message = fmt.Sprintf("no successful recall receipt in the last %d minutes; call memory_recall before continuing non-trivial work", withinMinutes)
	return out
}

// CheckReceipt returns whether account has a recent receipt for the external/session id.
func (s *Service) CheckReceipt(ctx context.Context, accountID uuid.UUID, externalSessionID string, sessionID *uuid.UUID, withinMinutes int) (ReceiptCheck, error) {
	withinMinutes = normalizeWithinMinutes(withinMinutes)
	ext := strings.TrimSpace(externalSessionID)
	if ext == "" && sessionID == nil {
		return ReceiptCheck{}, fmt.Errorf("recall: external_session_id or session_id required")
	}
	list, err := s.ListReceipts(ctx, accountID, ReceiptFilter{
		SessionID:         sessionID,
		ExternalSessionID: ext,
		Limit:             5,
	})
	if err != nil {
		return ReceiptCheck{}, err
	}
	return evaluateReceiptCheck(list, withinMinutes, time.Now().UTC()), nil
}

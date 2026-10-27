package agents

import "time"

var (
	heartbeatOnlineWindow = HeartbeatOnlineWindow
	activityOnlineWindow  = ActivityOnlineWindow
)

// ConfigureOnlineWindows overrides defaults when d > 0 (typically from env at app start).
func ConfigureOnlineWindows(heartbeat, activity time.Duration) {
	if heartbeat > 0 {
		heartbeatOnlineWindow = heartbeat
	}
	if activity > 0 {
		activityOnlineWindow = activity
	}
}

func ProcessOnline(now time.Time, lastHeartbeat *time.Time) bool {
	return lastHeartbeat != nil && now.Sub(lastHeartbeat.UTC()) <= heartbeatOnlineWindow
}

func ActivityFresh(now time.Time, lastIngest, lastRecall *time.Time) bool {
	for _, t := range []*time.Time{lastIngest, lastRecall} {
		if t != nil && now.Sub(t.UTC()) <= activityOnlineWindow {
			return true
		}
	}
	return false
}

func Online(now time.Time, lastHeartbeat, lastIngest, lastRecall *time.Time) bool {
	return ProcessOnline(now, lastHeartbeat) || ActivityFresh(now, lastIngest, lastRecall)
}

func LatestActivity(lastHeartbeat, lastIngest, lastRecall *time.Time) *time.Time {
	var best *time.Time
	for _, t := range []*time.Time{lastHeartbeat, lastIngest, lastRecall} {
		if t == nil {
			continue
		}
		if best == nil || t.After(*best) {
			best = t
		}
	}
	return best
}

package agents

import (
	"testing"
	"time"
)

func TestOnline_heartbeatWins(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	hb := now.Add(-30 * time.Second)
	if !Online(now, &hb, nil, nil) {
		t.Fatal("expected online via heartbeat")
	}
}

func TestOnline_activityWhenNoHeartbeat(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	ing := now.Add(-5 * time.Minute)
	if !Online(now, nil, &ing, nil) {
		t.Fatal("expected online via ingest")
	}
}

func TestOnline_offline(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	ing := now.Add(-20 * time.Minute)
	if Online(now, nil, &ing, nil) {
		t.Fatal("expected offline")
	}
}

func TestProcessOnline_and_ActivityFresh(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	hb := now.Add(-30 * time.Second)
	ing := now.Add(-20 * time.Minute)
	if !ProcessOnline(now, &hb) {
		t.Fatal("expected process online")
	}
	if ActivityFresh(now, &ing, nil) {
		t.Fatal("ingest too old")
	}
	if !Online(now, &hb, &ing, nil) {
		t.Fatal("online should OR")
	}
	fresh := now.Add(-2 * time.Minute)
	if !ActivityFresh(now, &fresh, nil) {
		t.Fatal("expected activity fresh")
	}
	if ProcessOnline(now, nil) {
		t.Fatal("nil hb offline")
	}
}

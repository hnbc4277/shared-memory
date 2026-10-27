//go:build integration

package agents_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"sharedmemory/internal/agents"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/device"
	"sharedmemory/internal/testutil"
)

func TestStore_TouchHeartbeat(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	store := agents.NewStore(pool, auth.NewStore(pool))

	created, err := store.CreateRegistered(ctx, acct, agents.CreateInput{
		Agent: "cursor", Label: "Heartbeat test",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Connection.ID
	if err := store.TouchHeartbeat(ctx, acct, id); err != nil {
		t.Fatal(err)
	}
	groups, err := store.ListGrouped(ctx, acct)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Instances) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	inst := groups[0].Instances[0]
	if !inst.ProcessOnline {
		t.Fatalf("expected process_online after TouchHeartbeat: %+v", inst)
	}
	if !inst.Online {
		t.Fatal("expected online via process_online")
	}
	if err := store.Revoke(ctx, acct, id); err != nil {
		t.Fatal(err)
	}
	if err := store.TouchHeartbeat(ctx, acct, id); err != agents.ErrNotFound {
		t.Fatalf("revoked TouchHeartbeat = %v want ErrNotFound", err)
	}
}

func TestStore_AutoIngestAndGroupedList(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	store := agents.NewStore(pool, auth.NewStore(pool))

	if err := store.RecordIngest(ctx, acct, nil, agents.Activity{
		Agent: "cursor", AgentPath: "cursor/researcher", Model: "gpt-4",
	}); err != nil {
		t.Fatal(err)
	}
	groups, err := store.ListGrouped(ctx, acct)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Agent != "cursor" {
		t.Fatalf("groups = %+v", groups)
	}
	if len(groups[0].Instances) != 1 || groups[0].Instances[0].Kind != agents.KindAuto {
		t.Fatalf("instances = %+v", groups[0].Instances)
	}
	if groups[0].Instances[0].LastIngestAt == nil {
		t.Fatal("expected last_ingest_at")
	}
}

func TestStore_RegisteredConnectionActivity(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	store := agents.NewStore(pool, auth.NewStore(pool))

	created, err := store.CreateRegistered(ctx, acct, agents.CreateInput{
		Agent: "codex", Label: "Office PC",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Connection.ID
	if err := store.RecordIngest(ctx, acct, &id, agents.Activity{Agent: "codex"}); err != nil {
		t.Fatal(err)
	}
	groups, err := store.ListGrouped(ctx, acct)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Instances) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	if groups[0].Instances[0].Kind != agents.KindRegistered {
		t.Fatalf("kind = %s", groups[0].Instances[0].Kind)
	}
}

func TestStore_UpgradeAuto(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	store := agents.NewStore(pool, auth.NewStore(pool))

	if err := store.RecordIngest(ctx, acct, nil, agents.Activity{
		Agent: "cursor", AgentPath: "cursor/main", Model: "gpt-4",
	}); err != nil {
		t.Fatal(err)
	}
	groups, err := store.ListGrouped(ctx, acct)
	if err != nil || len(groups) != 1 || len(groups[0].Instances) != 1 {
		t.Fatalf("auto setup: groups=%+v err=%v", groups, err)
	}
	autoID := groups[0].Instances[0].ID
	if groups[0].Instances[0].Kind != agents.KindAuto {
		t.Fatalf("kind = %s", groups[0].Instances[0].Kind)
	}

	up, err := store.UpgradeAuto(ctx, acct, autoID, "Office", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if up.Connection.Kind != agents.KindRegistered || up.Connection.Label != "Office" {
		t.Fatalf("upgraded = %+v", up.Connection)
	}

	groups, err = store.ListGrouped(ctx, acct)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Instances) != 1 {
		t.Fatalf("after upgrade groups = %+v", groups)
	}
	if groups[0].Instances[0].ID != up.Connection.ID {
		t.Fatalf("expected registered id %s got %s", up.Connection.ID, groups[0].Instances[0].ID)
	}
	if groups[0].Instances[0].LastAgentPath != "cursor/main" {
		t.Fatalf("activity not copied: %+v", groups[0].Instances[0])
	}
}

func TestStore_DeviceLinkedOnline(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	authStore := auth.NewStore(pool)
	devStore := device.NewStore(pool, authStore)
	store := agents.NewStore(pool, authStore)

	code, err := devStore.CreateCode(ctx, "smoke-daemon", "cursor", "http://127.0.0.1:5173")
	if err != nil {
		t.Fatal(err)
	}
	if err := devStore.Approve(ctx, acct, code.UserCode); err != nil {
		t.Fatal(err)
	}
	tok, err := devStore.Poll(ctx, code.DeviceCode)
	if err != nil {
		t.Fatal(err)
	}
	list, err := devStore.List(ctx, acct)
	if err != nil || len(list) != 1 {
		t.Fatalf("devices = %+v err=%v", list, err)
	}
	if err := devStore.HeartbeatDevice(ctx, acct, list[0].ID, "1.0.0", 2); err != nil {
		t.Fatal(err)
	}
	_ = tok

	groups, err := store.ListGrouped(ctx, acct)
	if err != nil {
		t.Fatal(err)
	}
	var linked *agents.Connection
	for _, g := range groups {
		for i := range g.Instances {
			if g.Instances[i].Kind == agents.KindDeviceLinked {
				linked = &g.Instances[i]
				break
			}
		}
	}
	if linked == nil {
		t.Fatal("expected device_linked instance")
	}
	if !linked.Online {
		t.Fatalf("expected online after heartbeat: %+v", linked)
	}
	if linked.DaemonVersion != "1.0.0" || linked.SpoolPending != 2 {
		t.Fatalf("device enrich = %+v", linked)
	}
}

func TestStore_ClaimHistory(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	store := agents.NewStore(pool, auth.NewStore(pool))

	sid := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (id, account_id, agent, agent_path, status, started_at, ended_at)
		VALUES ($1,$2,'cursor','cursor','active',now(),now())`, sid, acct); err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateRegistered(ctx, acct, agents.CreateInput{Agent: "cursor", Label: "Desk"})
	if err != nil {
		t.Fatal(err)
	}
	var unattached int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM sessions WHERE account_id = $1 AND agent = 'cursor' AND connection_id IS NULL`, acct).Scan(&unattached); err != nil {
		t.Fatal(err)
	}
	if unattached == 0 {
		t.Fatal("expected session without connection_id")
	}
	mid := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, session_id, account_id, role, seq, skills, model, agent, agent_path, attachment_ids, created_at)
		VALUES ($1,$2,$3,'user',1,'[]','','cursor','cursor','[]',now())`, mid, sid, acct); err != nil {
		t.Fatal(err)
	}
	n, err := store.ClaimHistory(ctx, acct, created.Connection.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(unattached) {
		t.Fatalf("claimed %d want %d", n, unattached)
	}
	var msgConn *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT connection_id FROM messages WHERE id = $1`, mid).Scan(&msgConn); err != nil {
		t.Fatal(err)
	}
	if msgConn == nil || *msgConn != created.Connection.ID {
		t.Fatalf("message connection_id = %v want %s", msgConn, created.Connection.ID)
	}
}

func TestStore_ConnectionIDForAPIKey(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	authStore := auth.NewStore(pool)
	store := agents.NewStore(pool, authStore)
	created, err := store.CreateRegistered(ctx, acct, agents.CreateInput{
		Agent: "cursor", Label: "Dedicated", IssueDedicatedKey: true,
	})
	if err != nil || created.APIKey == "" {
		t.Fatalf("create with key: err=%v key=%q", err, created.APIKey)
	}
	_, keyID, _, err := authStore.AuthenticateAPIKeyFull(ctx, created.APIKey)
	if err != nil {
		t.Fatal(err)
	}
	cid, err := store.ConnectionIDForAPIKey(ctx, acct, *keyID)
	if err != nil {
		t.Fatal(err)
	}
	if cid == nil || *cid != created.Connection.ID {
		t.Fatalf("ConnectionIDForAPIKey = %v want %s", cid, created.Connection.ID)
	}
}

func TestStore_ClaimHistoryBefore(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	store := agents.NewStore(pool, auth.NewStore(pool))

	recent := uuid.New()
	old := uuid.New()
	oldStart := time.Now().UTC().AddDate(0, 0, -30)
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (id, account_id, agent, agent_path, status, started_at, ended_at)
		VALUES ($1,$2,'cursor','cursor','active',now(),now())`, recent, acct); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (id, account_id, agent, agent_path, status, started_at, ended_at)
		VALUES ($1,$2,'cursor','cursor','active',$3,now())`, old, acct, oldStart); err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateRegistered(ctx, acct, agents.CreateInput{Agent: "cursor", Label: "Desk"})
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().AddDate(0, 0, -7)
	n, err := store.ClaimHistory(ctx, acct, created.Connection.ID, &before)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("claimed %d want 1 (only old session)", n)
	}
	var recentConn, oldConn *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT connection_id FROM sessions WHERE id = $1`, recent).Scan(&recentConn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT connection_id FROM sessions WHERE id = $1`, old).Scan(&oldConn); err != nil {
		t.Fatal(err)
	}
	if recentConn != nil {
		t.Fatal("recent session should remain unclaimed")
	}
	if oldConn == nil || *oldConn != created.Connection.ID {
		t.Fatalf("old session connection_id = %v", oldConn)
	}
}

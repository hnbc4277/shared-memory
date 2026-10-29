package recall

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/memory"
)

// memoryVisibleForConnection: unattributed memories stay shared; others must match the connection.
func memoryVisibleForConnection(m memory.Memory, connectionID uuid.UUID) bool {
	if m.SourceConnectionID == nil {
		return true
	}
	return *m.SourceConnectionID == connectionID
}

func sessionConnectionMap(ctx context.Context, pool *pgxpool.Pool, accountID uuid.UUID, sessionIDs []uuid.UUID) (map[uuid.UUID]*uuid.UUID, error) {
	out := map[uuid.UUID]*uuid.UUID{}
	if pool == nil || len(sessionIDs) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT id, connection_id FROM sessions
		WHERE account_id = $1 AND id = ANY($2)`, accountID, sessionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var cid *uuid.UUID
		if err := rows.Scan(&sid, &cid); err != nil {
			return nil, err
		}
		out[sid] = cid
	}
	return out, rows.Err()
}

func artifactVisibleForConnection(sourceSessionID *uuid.UUID, bySession map[uuid.UUID]*uuid.UUID, connectionID uuid.UUID) bool {
	if sourceSessionID == nil {
		return true
	}
	cid, ok := bySession[*sourceSessionID]
	if !ok || cid == nil {
		return true
	}
	return *cid == connectionID
}

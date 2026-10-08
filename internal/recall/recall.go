package recall

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/entity"
	"sharedmemory/internal/index"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/semantic"
)

const (
	DefaultTimeout = time.Second
	DefaultLimit   = 8

	WarnIndexDegraded = "index_degraded"
	WarnPGFallback    = "pg_fallback"
	WarnInjectOff     = "inject_disabled"
)

type Request struct {
	Query             string     `json:"query"`
	ProjectID         *uuid.UUID `json:"project_id"`
	TopK              int        `json:"top_k"`
	SessionID         *uuid.UUID `json:"session_id,omitempty"`
	ExternalSessionID string     `json:"external_session_id,omitempty"`
	Agent             string     `json:"agent,omitempty"`
	AgentPath         string     `json:"agent_path,omitempty"`
	Source            string     `json:"source,omitempty"`
}

type Item struct {
	ID              uuid.UUID  `json:"id"`
	DocType         string     `json:"doc_type"`
	Kind            string     `json:"kind"`
	Scope           string     `json:"scope"`
	Title           string     `json:"title,omitempty"`
	Body            string     `json:"body"`
	SourceSessionID *uuid.UUID `json:"source_session_id"`
	SourceAgentPath string     `json:"source_agent_path"`
	CreatedAt       time.Time  `json:"created_at"`
	Score           float64    `json:"score"`
}

type Result struct {
	Profile        string    `json:"profile"`
	RunningSummary string    `json:"running_summary"`
	Hits           []Item    `json:"hits"`
	Context        string    `json:"context"`
	InjectEnabled  bool      `json:"inject_enabled"`
	Warnings       []string  `json:"warnings"`
	ReceiptID      uuid.UUID `json:"receipt_id,omitempty"`
}

// Service never reads vault tables: secrets are not part of any recall path.
type Service struct {
	Pool     *pgxpool.Pool
	Index    *index.Service
	Memories *memory.Store
	Policies *policy.Store
	Entities *entity.Store
	Timeout  time.Duration
}

func (s *Service) Recall(ctx context.Context, accountID uuid.UUID, req Request) (Result, error) {
	res := Result{Hits: []Item{}, Warnings: []string{}}
	if req.TopK <= 0 || req.TopK > 50 {
		req.TopK = DefaultLimit
	}
	pol, err := s.Policies.Get(ctx, accountID)
	if err != nil {
		return res, err
	}
	res.InjectEnabled = pol.InjectEnabled
	if !pol.InjectEnabled {
		res.Warnings = append(res.Warnings, WarnInjectOff)
	}
	if res.Profile, _, err = s.Memories.Profile(ctx, accountID); err != nil {
		return res, err
	}
	if req.ProjectID != nil {
		mems, err := s.Memories.List(ctx, accountID, memory.Filter{ProjectID: req.ProjectID,
			Kind: semantic.KindRunningSummary, Scope: semantic.ScopeProject, Limit: 1})
		if err != nil {
			return res, err
		}
		if len(mems) > 0 {
			res.RunningSummary = mems[0].Body
		}
	}

	if q := strings.TrimSpace(req.Query); q != "" {
		searchQ := expandQuery(ctx, s.Entities, accountID, req.ProjectID, q)
		var hits []index.Hit
		var searchErr error
		if s.Index != nil {
			timeout := s.Timeout
			if timeout <= 0 {
				timeout = DefaultTimeout
			}
			sctx, cancel := context.WithTimeout(ctx, timeout)
			hits, searchErr = s.Index.Search(sctx, index.Query{AccountID: accountID, ProjectID: req.ProjectID,
				Text: searchQ, Limit: req.TopK * 2})
			cancel()
			if searchErr != nil {
				log.Printf("recall search: %v", searchErr)
				res.Warnings = append(res.Warnings, WarnIndexDegraded)
			}
		}
		items, err := s.hydrate(ctx, accountID, req.ProjectID, hits, pol.ArtifactSearchable)
		if err != nil {
			return res, err
		}
		if len(items) == 0 {
			res.Warnings = append(res.Warnings, WarnPGFallback)
			mems, err := s.Memories.Search(ctx, accountID, req.ProjectID, searchQ, req.TopK)
			if err != nil {
				return res, err
			}
			for _, m := range mems {
				items = append(items, fromMemory(m, 0))
			}
		}
		if len(items) > req.TopK {
			items = items[:req.TopK]
		}
		res.Hits = items
	}
	if pol.InjectEnabled {
		res.Context = format(res)
	}
	if id, err := s.recordReceipt(ctx, accountID, req, res); err != nil {
		log.Printf("recall receipt: %v", err)
	} else {
		res.ReceiptID = id
	}
	return res, nil
}

func fromMemory(m memory.Memory, score float64) Item {
	return Item{ID: m.ID, DocType: index.DocMemory, Kind: m.Kind, Scope: m.Scope, Body: m.Body,
		SourceSessionID: m.SourceSessionID, SourceAgentPath: m.SourceAgentPath, CreatedAt: m.CreatedAt, Score: score}
}

// expandQuery resolves entity aliases so "OS" can match memories about "OpenSearch".
func expandQuery(ctx context.Context, entities *entity.Store, accountID uuid.UUID, projectID *uuid.UUID, q string) string {
	if entities == nil {
		return q
	}
	canon, err := entities.Resolve(ctx, accountID, projectID, q)
	if err != nil || canon == "" || canon == q {
		return q
	}
	return canon + " " + q
}

// hydrate re-reads every hit from Postgres so stale projections (deleted or
// invalidated rows, other accounts) can never leak into results.
func (s *Service) hydrate(ctx context.Context, accountID uuid.UUID, projectID *uuid.UUID, hits []index.Hit, artifacts bool) ([]Item, error) {
	var memIDs, artIDs []uuid.UUID
	score := map[uuid.UUID]float64{}
	for _, h := range hits {
		score[h.ID] = h.Score
		if h.DocType == index.DocArtifact {
			artIDs = append(artIDs, h.ID)
		} else {
			memIDs = append(memIDs, h.ID)
		}
	}
	byID := map[uuid.UUID]Item{}
	mems, err := s.Memories.GetMany(ctx, accountID, memIDs)
	if err != nil {
		return nil, err
	}
	for _, m := range mems {
		if m.ProjectID != nil && (projectID == nil || *m.ProjectID != *projectID) {
			continue
		}
		byID[m.ID] = fromMemory(m, score[m.ID])
	}
	if artifacts && len(artIDs) > 0 {
		rows, err := s.Pool.Query(ctx, `SELECT id, project_id, title, type, summary, source_session_id,
			source_agent_path, created_at FROM artifacts WHERE account_id = $1 AND id = ANY($2)`, accountID, artIDs)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var it Item
			var pid *uuid.UUID
			if err := rows.Scan(&it.ID, &pid, &it.Title, &it.Kind, &it.Body, &it.SourceSessionID,
				&it.SourceAgentPath, &it.CreatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			if pid != nil && (projectID == nil || *pid != *projectID) {
				continue
			}
			it.DocType = index.DocArtifact
			it.Scope = semantic.ScopeGlobal
			if pid != nil {
				it.Scope = semantic.ScopeProject
			}
			it.Score = score[it.ID]
			byID[it.ID] = it
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]Item, 0, len(byID))
	for _, h := range hits {
		if it, ok := byID[h.ID]; ok {
			out = append(out, it)
			delete(byID, h.ID)
		}
	}
	return out, nil
}

// format renders the block adapters inject into the agent's context.
func format(res Result) string {
	if strings.TrimSpace(res.Profile) == "" && res.RunningSummary == "" && len(res.Hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 共享记忆（shared-memory）\n")
	if strings.TrimSpace(res.Profile) != "" {
		b.WriteString("### 用户画像\n")
		b.WriteString(res.Profile)
		b.WriteString("\n")
	}
	if res.RunningSummary != "" {
		b.WriteString("### 项目近况\n")
		b.WriteString(res.RunningSummary)
		b.WriteString("\n")
	}
	if len(res.Hits) > 0 {
		b.WriteString("### 相关记忆\n")
		for _, it := range res.Hits {
			label := it.Kind
			if it.DocType == index.DocArtifact {
				label = "成稿:" + it.Title
			}
			fmt.Fprintf(&b, "- [%s · %s · %s · %s] %s\n", label, it.Scope, it.SourceAgentPath,
				it.CreatedAt.Format("2006-01-02"), it.Body)
		}
	}
	return b.String()
}

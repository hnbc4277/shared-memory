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
	"sharedmemory/internal/kb"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/semantic"
)

const (
	DefaultTimeout = time.Second
	MaxMemoryHits  = 5
	MaxKBHits      = 3
	DefaultLimit   = MaxMemoryHits + MaxKBHits

	WarnIndexDegraded = "index_degraded"
	WarnPGFallback    = "pg_fallback"
	WarnInjectOff     = "inject_disabled"
	WarnKBDegraded    = "kb_degraded"
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
	ConnectionID      string     `json:"connection_id,omitempty"`
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

type KBHit struct {
	ID         uuid.UUID `json:"id"`
	DocumentID uuid.UUID `json:"document_id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Score      float64   `json:"score"`
}

type Result struct {
	Profile        string    `json:"profile"`
	RunningSummary string    `json:"running_summary"`
	Hits           []Item    `json:"hits"`
	KBHits         []KBHit   `json:"kb_hits"`
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
	KB       *kb.Store
	Timeout  time.Duration
}

func (s *Service) Recall(ctx context.Context, accountID uuid.UUID, req Request) (Result, error) {
	res := Result{Hits: []Item{}, KBHits: []KBHit{}, Warnings: []string{}}
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
	wantKB := pol.InjectEnabled && pol.KBInjectEnabled && s.KB != nil
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
			limit := req.TopK * 2
			if min := MaxMemoryHits + MaxKBHits; limit < min {
				limit = min
			}
			sctx, cancel := context.WithTimeout(ctx, timeout)
			hits, searchErr = s.Index.Search(sctx, index.Query{AccountID: accountID, ProjectID: req.ProjectID,
				Text: searchQ, Limit: limit})
			cancel()
			if searchErr != nil {
				log.Printf("recall search: %v", searchErr)
				res.Warnings = append(res.Warnings, WarnIndexDegraded)
			}
		}
		items, kbHits, kbDegraded, err := s.hydrate(ctx, accountID, req.ProjectID, hits, pol.ArtifactSearchable, wantKB)
		if err != nil {
			return res, err
		}
		if kbDegraded {
			res.Warnings = append(res.Warnings, WarnKBDegraded)
		}
		if len(items) == 0 {
			if len(kbHits) == 0 {
				res.Warnings = append(res.Warnings, WarnPGFallback)
			}
			mems, err := s.Memories.Search(ctx, accountID, req.ProjectID, searchQ, req.TopK)
			if err != nil {
				return res, err
			}
			for _, m := range mems {
				items = append(items, fromMemory(m, 0))
			}
		}
		memCap := MaxMemoryHits
		if req.TopK < memCap {
			memCap = req.TopK
		}
		if n := len(kbHits); n < MaxKBHits {
			memCap += MaxKBHits - n
		}
		if len(items) > memCap {
			items = items[:memCap]
		}
		if len(kbHits) > MaxKBHits {
			kbHits = kbHits[:MaxKBHits]
		}
		res.Hits = items
		res.KBHits = kbHits
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
// KB chunk IDs are never passed to memory GetMany; failures hydrating KB only
// degrade kb_hits and never block memory/artifact results.
func (s *Service) hydrate(ctx context.Context, accountID uuid.UUID, projectID *uuid.UUID, hits []index.Hit, artifacts, wantKB bool) ([]Item, []KBHit, bool, error) {
	var memIDs, artIDs, kbIDs []uuid.UUID
	score := map[uuid.UUID]float64{}
	for _, h := range hits {
		score[h.ID] = h.Score
		switch h.DocType {
		case index.DocArtifact:
			artIDs = append(artIDs, h.ID)
		case index.DocKBChunk:
			if wantKB {
				kbIDs = append(kbIDs, h.ID)
			}
		default:
			memIDs = append(memIDs, h.ID)
		}
	}
	byID := map[uuid.UUID]Item{}
	mems, err := s.Memories.GetMany(ctx, accountID, memIDs)
	if err != nil {
		return nil, nil, false, err
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
			return nil, nil, false, err
		}
		for rows.Next() {
			var it Item
			var pid *uuid.UUID
			if err := rows.Scan(&it.ID, &pid, &it.Title, &it.Kind, &it.Body, &it.SourceSessionID,
				&it.SourceAgentPath, &it.CreatedAt); err != nil {
				rows.Close()
				return nil, nil, false, err
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
			return nil, nil, false, err
		}
	}
	out := make([]Item, 0, len(byID))
	for _, h := range hits {
		if it, ok := byID[h.ID]; ok {
			out = append(out, it)
			delete(byID, h.ID)
		}
	}
	kbHits, kbErr := s.hydrateKB(ctx, accountID, kbIDs, score)
	if kbErr != nil {
		log.Printf("recall kb hydrate: %v", kbErr)
		return out, []KBHit{}, true, nil
	}
	return out, kbHits, false, nil
}

func (s *Service) hydrateKB(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID, score map[uuid.UUID]float64) ([]KBHit, error) {
	if s.KB == nil || len(ids) == 0 {
		return []KBHit{}, nil
	}
	chunks, err := s.KB.GetManyChunks(ctx, accountID, ids)
	if err != nil {
		return []KBHit{}, err
	}
	docIDs := make([]uuid.UUID, 0, len(chunks))
	seen := map[uuid.UUID]bool{}
	for _, c := range chunks {
		if !seen[c.DocumentID] {
			seen[c.DocumentID] = true
			docIDs = append(docIDs, c.DocumentID)
		}
	}
	type docMeta struct {
		title  string
		status string
	}
	meta := map[uuid.UUID]docMeta{}
	if len(docIDs) > 0 {
		rows, err := s.Pool.Query(ctx, `
			SELECT id, COALESCE(title,''), status
			FROM kb_documents
			WHERE account_id = $1 AND id = ANY($2)`, accountID, docIDs)
		if err != nil {
			return []KBHit{}, err
		}
		for rows.Next() {
			var id uuid.UUID
			var m docMeta
			if err := rows.Scan(&id, &m.title, &m.status); err != nil {
				rows.Close()
				return []KBHit{}, err
			}
			meta[id] = m
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return []KBHit{}, err
		}
	}
	byID := map[uuid.UUID]KBHit{}
	for _, c := range chunks {
		m, ok := meta[c.DocumentID]
		if !ok || m.status != kb.StatusReady {
			continue
		}
		byID[c.ID] = KBHit{
			ID: c.ID, DocumentID: c.DocumentID, Title: m.title, Body: c.Body, Score: score[c.ID],
		}
	}
	out := make([]KBHit, 0, len(byID))
	for _, id := range ids {
		if h, ok := byID[id]; ok {
			out = append(out, h)
			delete(byID, id)
		}
	}
	return out, nil
}

// format renders the block adapters inject into the agent's context.
func format(res Result) string {
	if strings.TrimSpace(res.Profile) == "" && res.RunningSummary == "" && len(res.Hits) == 0 && len(res.KBHits) == 0 {
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
	if len(res.KBHits) > 0 {
		b.WriteString("## Knowledge Base\n")
		for _, h := range res.KBHits {
			fmt.Fprintf(&b, "- [%s] %s\n", h.Title, h.Body)
		}
	}
	return b.String()
}

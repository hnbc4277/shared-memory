package httpapi

import (
	"net/http"
	"strconv"

	"sharedmemory/internal/audit"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/recall"
)

func (r *Router) handleRecall(w http.ResponseWriter, req *http.Request) {
	var body recall.Request
	if !decode(w, req, &body) {
		return
	}
	if body.ProjectID != nil {
		if _, err := r.Projects.Get(req.Context(), accountOf(req), *body.ProjectID); err != nil {
			fail(w, err)
			return
		}
	}
	res, err := r.Recall.Recall(req.Context(), accountOf(req), body)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (r *Router) handleListMemories(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	projectID, ok := optionalUUID(w, q.Get("project_id"), "project_id")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	list, err := r.Memories.List(req.Context(), accountOf(req), memory.Filter{
		ProjectID: projectID, Kind: q.Get("kind"), Scope: q.Get("scope"),
		IncludeInvalidated: q.Get("include_invalidated") == "true", Limit: limit, Offset: offset,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type supersedeReq struct {
	Body string `json:"body"`
}

func (r *Router) handleSupersedeMemory(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var body supersedeReq
	if !decode(w, req, &body) {
		return
	}
	if body.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}
	m, err := r.Memories.Supersede(req.Context(), accountOf(req), id, body.Body)
	if err != nil {
		fail(w, err)
		return
	}
	r.refreshProfile(req, m.Kind)
	r.audit(req, audit.MemorySupersede, map[string]any{"memory_id": id, "new_id": m.ID})
	writeJSON(w, http.StatusOK, m)
}

func (r *Router) handleInvalidateMemory(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	m, err := r.Memories.Get(req.Context(), accountOf(req), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := r.Memories.Invalidate(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	r.refreshProfile(req, m.Kind)
	r.audit(req, audit.MemoryInvalid, map[string]any{"memory_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) refreshProfile(req *http.Request, kind string) {
	if kind == "preference" {
		_, _ = r.Memories.RebuildProfile(req.Context(), accountOf(req))
	}
}

func (r *Router) handleGetProfile(w http.ResponseWriter, req *http.Request) {
	body, at, err := r.Memories.Profile(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"body": body, "updated_at": at})
}

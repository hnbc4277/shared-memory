package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"sharedmemory/internal/attachment"
	"sharedmemory/internal/session"
)

func (r *Router) handleIngest(w http.ResponseWriter, req *http.Request) {
	var body session.IngestMessageRequest
	if !decode(w, req, &body) {
		return
	}
	accountID := accountOf(req)
	resolved, ok := r.resolveConnectionID(w, req, accountID, body.ConnectionID)
	if !ok {
		return
	}
	body.ConnectionID = resolved
	res, err := r.Sessions.Ingest(req.Context(), accountID, body)
	if err != nil {
		if errors.Is(err, session.ErrInvalid) && r.Quarantine != nil {
			aid := accountID
			_ = r.Quarantine.Add(req.Context(), &aid, body.Agent, body.AgentPath, err.Error(), body)
		}
		fail(w, err)
		return
	}
	r.recordIngestAgents(req, accountID, body)
	writeJSON(w, http.StatusCreated, res)
}

func parseTime(w http.ResponseWriter, raw, field string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, field+" must be RFC3339")
		return nil, false
	}
	return &t, true
}

func (r *Router) handleListSessions(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	projectID, ok := optionalUUID(w, q.Get("project_id"), "project_id")
	if !ok {
		return
	}
	from, ok := parseTime(w, q.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := parseTime(w, q.Get("to"), "to")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	connectionID, ok := optionalUUID(w, q.Get("connection_id"), "connection_id")
	if !ok {
		return
	}
	list, err := r.Sessions.List(req.Context(), accountOf(req), session.ListFilter{
		ProjectID: projectID, ConnectionID: connectionID, Agent: q.Get("agent"), AgentPath: q.Get("agent_path"),
		From: from, To: to, Limit: limit, Offset: offset,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleGetSession(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	d, err := r.Sessions.Get(req.Context(), accountOf(req), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (r *Router) handleUploadAttachment(w http.ResponseWriter, req *http.Request) {
	req.Body = http.MaxBytesReader(w, req.Body, attachment.MaxBytes+1<<20)
	if err := req.ParseMultipartForm(4 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, attachment.ErrTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, "expected multipart/form-data with a 'file' field")
		return
	}
	file, header, err := req.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'file' field")
		return
	}
	defer file.Close()
	projectID, ok := optionalUUID(w, req.FormValue("project_id"), "project_id")
	if !ok {
		return
	}
	a, warnings, err := r.Attachments.Upload(req.Context(), accountOf(req), attachment.Upload{
		ProjectID: projectID, AgentPath: req.FormValue("agent_path"), Filename: header.Filename,
		MIME: header.Header.Get("Content-Type"), Body: file,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachment": a, "warnings": warnings})
}

func (r *Router) handleGetAttachment(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	a, url, err := r.Attachments.DownloadURL(req.Context(), accountOf(req), id)
	if errors.Is(err, attachment.ErrNotStored) {
		writeJSON(w, http.StatusOK, map[string]any{"attachment": a, "download_url": nil})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachment": a, "download_url": url})
}

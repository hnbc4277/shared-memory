package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"sharedmemory/internal/kb"
)

func (r *Router) handleCreateKBDocument(w http.ResponseWriter, req *http.Request) {
	if r.KB == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base unavailable")
		return
	}
	ct := req.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		r.handleUploadKBDocument(w, req)
		return
	}
	var body struct {
		ArtifactID   string `json:"artifact_id"`
		AttachmentID string `json:"attachment_id"`
	}
	if !decode(w, req, &body) {
		return
	}
	accountID := accountOf(req)
	switch {
	case strings.TrimSpace(body.ArtifactID) != "" && strings.TrimSpace(body.AttachmentID) != "":
		writeError(w, http.StatusBadRequest, "provide only one of artifact_id or attachment_id")
		return
	case strings.TrimSpace(body.ArtifactID) != "":
		id, ok := optionalUUID(w, body.ArtifactID, "artifact_id")
		if !ok || id == nil {
			return
		}
		doc, err := r.KB.AddFromArtifact(req.Context(), accountID, *id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"document": doc})
	case strings.TrimSpace(body.AttachmentID) != "":
		id, ok := optionalUUID(w, body.AttachmentID, "attachment_id")
		if !ok || id == nil {
			return
		}
		doc, err := r.KB.AddFromAttachment(req.Context(), accountID, *id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"document": doc})
	default:
		writeError(w, http.StatusBadRequest, "expected multipart file upload or JSON artifact_id/attachment_id")
	}
}

func (r *Router) handleUploadKBDocument(w http.ResponseWriter, req *http.Request) {
	req.Body = http.MaxBytesReader(w, req.Body, kb.MaxBytes+1<<20)
	if err := req.ParseMultipartForm(4 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, kb.ErrTooLarge)
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
	data, err := io.ReadAll(io.LimitReader(file, kb.MaxBytes+1))
	if err != nil {
		fail(w, err)
		return
	}
	doc, err := r.KB.AddUpload(req.Context(), accountOf(req), kb.AddUploadInput{
		Title: req.FormValue("title"),
		MIME:  header.Header.Get("Content-Type"),
		Name:  header.Filename,
		Body:  data,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"document": doc})
}

func (r *Router) handleListKBDocuments(w http.ResponseWriter, req *http.Request) {
	if r.KB == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base unavailable")
		return
	}
	q := req.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	list, err := r.KB.List(req.Context(), accountOf(req), limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	if list == nil {
		list = []kb.Document{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": list})
}

func (r *Router) handleGetKBDocument(w http.ResponseWriter, req *http.Request) {
	if r.KB == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	doc, err := r.KB.Get(req.Context(), accountOf(req), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": doc})
}

func (r *Router) handleDeleteKBDocument(w http.ResponseWriter, req *http.Request) {
	if r.KB == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.KB.Delete(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleReparseKBDocument(w http.ResponseWriter, req *http.Request) {
	if r.KB == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	doc, err := r.KB.Reparse(req.Context(), accountOf(req), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": doc})
}

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"sharedmemory/internal/device"
)

func (r *Router) handleDeviceCode(w http.ResponseWriter, req *http.Request) {
	var body struct {
		ClientName string `json:"client_name"`
		Agent      string `json:"agent"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	base := os.Getenv("SHARED_MEMORY_WEB_URL")
	if base == "" {
		base = "http://127.0.0.1:5173"
	}
	code, err := r.Devices.CreateCode(req.Context(), body.ClientName, body.Agent, base)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, code)
}

func (r *Router) handleDeviceToken(w http.ResponseWriter, req *http.Request) {
	var body struct {
		DeviceCode string `json:"device_code"`
	}
	if !decode(w, req, &body) {
		return
	}
	tok, err := r.Devices.Poll(req.Context(), body.DeviceCode)
	if errors.Is(err, device.ErrPending) {
		writeError(w, http.StatusBadRequest, "authorization_pending")
		return
	}
	if errors.Is(err, device.ErrExpired) {
		writeError(w, http.StatusBadRequest, "expired_token")
		return
	}
	if errors.Is(err, device.ErrRedeemed) {
		writeError(w, http.StatusBadRequest, "already_redeemed")
		return
	}
	if errors.Is(err, device.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tok)
}

func (r *Router) handleDeviceApprove(w http.ResponseWriter, req *http.Request) {
	var body struct {
		UserCode string `json:"user_code"`
	}
	if !decode(w, req, &body) {
		return
	}
	code := device.NormalizeUserCode(body.UserCode)
	err := r.Devices.Approve(req.Context(), accountOf(req), code)
	if errors.Is(err, device.ErrNotFound) || errors.Is(err, device.ErrInvalid) {
		writeError(w, http.StatusNotFound, "unknown user_code")
		return
	}
	if errors.Is(err, device.ErrExpired) {
		writeError(w, http.StatusBadRequest, "expired_token")
		return
	}
	if errors.Is(err, device.ErrRedeemed) {
		writeError(w, http.StatusConflict, "already used")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	r.audit(req, "device.approve", map[string]any{"user_code": code})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleListDevices(w http.ResponseWriter, req *http.Request) {
	list, err := r.Devices.List(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleRevokeDevice(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Devices.Revoke(req.Context(), accountOf(req), id); err != nil {
		if errors.Is(err, device.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		fail(w, err)
		return
	}
	r.audit(req, "device.revoke", map[string]any{"device_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleDeviceHeartbeat(w http.ResponseWriter, req *http.Request) {
	var body struct {
		DaemonVersion string `json:"daemon_version"`
		SpoolPending  int    `json:"spool_pending"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	deviceID, ok := DeviceIDFromContext(req.Context())
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := r.Devices.HeartbeatDevice(req.Context(), accountOf(req), deviceID, body.DaemonVersion, body.SpoolPending); err != nil {
		if errors.Is(err, device.ErrNotFound) {
			writeError(w, http.StatusNotFound, "device revoked")
			return
		}
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

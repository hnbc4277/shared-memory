package session

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid ingest request")

const UnknownModel = "unknown"

var allowedRoles = map[string]bool{"user": true, "assistant": true, "system": true, "tool": true}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// ValidateIngest enforces the mandatory provenance contract. It never mutates req.
func ValidateIngest(req IngestMessageRequest) error {
	agent := strings.TrimSpace(req.Agent)
	if agent == "" {
		return invalid("agent is required")
	}
	if strings.Contains(agent, "/") {
		return invalid("agent must not contain '/'; use agent_path for sub-agents")
	}
	if path := strings.TrimSpace(req.AgentPath); path != "" {
		if path != agent && !strings.HasPrefix(path, agent+"/") {
			return invalid("agent_path %q must equal agent or start with %q", path, agent+"/")
		}
		for _, seg := range strings.Split(path, "/") {
			if strings.TrimSpace(seg) == "" {
				return invalid("agent_path %q has an empty segment", path)
			}
		}
	}
	if req.Skills == nil {
		return invalid("skills is required (use [] when none were used)")
	}
	if req.Plugins == nil {
		return invalid("plugins is required (use [] when none were used)")
	}
	if req.Tools == nil {
		return invalid("tools is required (use [] when none were used)")
	}
	if !allowedRoles[req.Role] {
		return invalid("role must be one of user|assistant|system|tool")
	}
	if strings.TrimSpace(req.Content) == "" && len(req.AttachmentIDs) == 0 {
		return invalid("content or attachment_ids is required")
	}
	if req.SessionID != "" {
		if _, err := uuid.Parse(req.SessionID); err != nil {
			return invalid("session_id must be a uuid")
		}
	}
	if ext := strings.TrimSpace(req.ExternalSessionID); ext != "" {
		if len(ext) > 256 {
			return invalid("external_session_id must be at most 256 characters")
		}
	}
	if req.ProjectID != "" {
		if _, err := uuid.Parse(req.ProjectID); err != nil {
			return invalid("project_id must be a uuid")
		}
	}
	for _, id := range req.AttachmentIDs {
		if _, err := uuid.Parse(id); err != nil {
			return invalid("attachment_ids must be uuids")
		}
	}
	return nil
}

// Normalize fills derived provenance defaults after validation.
func Normalize(req IngestMessageRequest) IngestMessageRequest {
	req.Agent = strings.TrimSpace(req.Agent)
	req.AgentPath = strings.TrimSpace(req.AgentPath)
	req.ExternalSessionID = strings.TrimSpace(req.ExternalSessionID)
	if req.AgentPath == "" {
		req.AgentPath = req.Agent
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = UnknownModel
	}
	req.Skills = cleanList(req.Skills)
	req.Plugins = cleanList(req.Plugins)
	req.Tools = cleanList(req.Tools)
	return req
}

func cleanList(in *[]string) *[]string {
	out := []string{}
	if in != nil {
		for _, s := range *in {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return &out
}

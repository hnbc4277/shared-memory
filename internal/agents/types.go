package agents

import (
	"time"

	"github.com/google/uuid"
)

const (
	KindAuto         = "auto"
	KindRegistered   = "registered"
	KindDeviceLinked = "device_linked"

	DefaultAutoLabel = "未绑定 · API Key"

	HeartbeatOnlineWindow = 90 * time.Second
	ActivityOnlineWindow  = 15 * time.Minute
)

type Connection struct {
	ID                    uuid.UUID  `json:"id"`
	Agent                 string     `json:"agent"`
	Label                 string     `json:"label"`
	Kind                  string     `json:"kind"`
	DeviceID              *uuid.UUID `json:"device_id,omitempty"`
	Online                bool       `json:"online"`
	ProcessOnline         bool       `json:"process_online"`
	ActivityFresh         bool       `json:"activity_fresh"`
	LastIngestAt          *time.Time `json:"last_ingest_at,omitempty"`
	LastRecallAt          *time.Time `json:"last_recall_at,omitempty"`
	LastHeartbeatAt       *time.Time `json:"last_heartbeat_at,omitempty"`
	LastAgentPath         string     `json:"last_agent_path,omitempty"`
	LastModel             string     `json:"last_model,omitempty"`
	LastExternalSessionID string     `json:"last_external_session_id,omitempty"`
	ClientName            string     `json:"client_name,omitempty"`
	DaemonVersion         string     `json:"daemon_version,omitempty"`
	SpoolPending          int        `json:"spool_pending,omitempty"`
	APIKeyName            string     `json:"api_key_name,omitempty"`
	HasSetupScript        bool       `json:"has_setup_script,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

type Group struct {
	Agent            string       `json:"agent"`
	DisplayName      string       `json:"display_name"`
	InstanceCount    int          `json:"instance_count"`
	OnlineCount      int          `json:"online_count"`
	LastActivityAt   *time.Time   `json:"last_activity_at,omitempty"`
	Instances        []Connection `json:"instances"`
}

type Activity struct {
	Agent               string
	AgentPath           string
	Model               string
	ExternalSessionID   string
}

type CreateInput struct {
	Agent              string
	Label              string
	IssueDedicatedKey  bool
}

type CreateResult struct {
	Connection Connection `json:"connection"`
	APIKey     string     `json:"api_key,omitempty"`
}

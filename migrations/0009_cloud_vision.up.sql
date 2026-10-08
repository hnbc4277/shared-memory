-- Account policy: allow cloud/multimodal vision for attachment descriptions.

ALTER TABLE account_policies
    ADD COLUMN IF NOT EXISTS cloud_vision_enabled BOOLEAN NOT NULL DEFAULT TRUE;

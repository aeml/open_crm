-- 1.4.8 Lead capture chat/website widget foundation.
-- Widgets reuse existing lead capture forms and expose a stable public ID for
-- iframe embeds. Live chat, bot flows, and agent routing remain future slices.

CREATE TABLE lead_chat_widgets (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  public_id TEXT NOT NULL UNIQUE,
  lead_capture_form_id BIGINT NOT NULL,
  name TEXT NOT NULL,
  title TEXT NOT NULL,
  welcome_message TEXT NOT NULL DEFAULT '',
  prompt_label TEXT NOT NULL DEFAULT 'Chat with us',
  cta_label TEXT NOT NULL DEFAULT 'Send',
  theme TEXT NOT NULL DEFAULT 'light',
  position TEXT NOT NULL DEFAULT 'bottom-right',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_chat_widgets_form_org_fk FOREIGN KEY (organization_id, lead_capture_form_id) REFERENCES lead_capture_forms(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT lead_chat_widgets_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT lead_chat_widgets_title_check CHECK (length(trim(title)) > 0),
  CONSTRAINT lead_chat_widgets_prompt_label_check CHECK (length(trim(prompt_label)) > 0),
  CONSTRAINT lead_chat_widgets_cta_label_check CHECK (length(trim(cta_label)) > 0),
  CONSTRAINT lead_chat_widgets_theme_check CHECK (theme IN ('light', 'blue', 'dark')),
  CONSTRAINT lead_chat_widgets_position_check CHECK (position IN ('bottom-right', 'bottom-left', 'inline'))
);

CREATE INDEX idx_lead_chat_widgets_org_active
  ON lead_chat_widgets(organization_id, is_active, updated_at DESC, id DESC);

CREATE INDEX idx_lead_chat_widgets_org_form
  ON lead_chat_widgets(organization_id, lead_capture_form_id);

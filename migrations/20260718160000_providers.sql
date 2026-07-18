CREATE TABLE "public"."providers" (
  "id" text NOT NULL,
  "kind" text NOT NULL,
  "base_url" text NOT NULL,
  "auth_mode" text NULL,
  "api_key_ref" text NULL,
  "region" text NULL,
  "enabled" boolean NULL,
  "max_rpm" bigint NULL,
  "spend_cap_usd_day" double precision NULL,
  "cost_in_per_m_tok" double precision NULL,
  "cost_out_per_m_tok" double precision NULL,
  "cost_per_hour" double precision NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE "public"."provider_model_map" (
  "provider_id" text NOT NULL,
  "base_model" text NOT NULL,
  "adapter" text NOT NULL DEFAULT '',
  "provider_model" text NOT NULL,
  "max_context" bigint NULL,
  PRIMARY KEY ("provider_id", "base_model", "adapter")
);

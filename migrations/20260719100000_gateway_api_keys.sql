CREATE TABLE "public"."gateway_api_keys" (
  "id" text NOT NULL,
  "key_hash" text NOT NULL,
  "client_id" text NOT NULL,
  "max_concurrent" bigint NULL DEFAULT 32,
  "enabled" boolean NOT NULL DEFAULT true,
  "created_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX "idx_gateway_api_keys_key_hash" ON "public"."gateway_api_keys" ("key_hash");
CREATE INDEX "idx_gateway_api_keys_client_id" ON "public"."gateway_api_keys" ("client_id");

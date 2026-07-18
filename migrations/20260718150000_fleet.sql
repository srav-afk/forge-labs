CREATE TABLE "public"."fleet_scaling_policies" (
  "id" bigserial NOT NULL,
  "base_model" text NOT NULL,
  "adapter" text NULL,
  "min_replicas" bigint NULL,
  "max_replicas" bigint NULL,
  "target_concurrency" bigint NULL,
  "scale_up_utilization" double precision NULL,
  "scale_down_delay_seconds" bigint NULL,
  "stabilization_window_seconds" bigint NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX "idx_fleet_model_identity" ON "public"."fleet_scaling_policies" ("base_model", "adapter");

ALTER TABLE "public"."workers" ADD COLUMN IF NOT EXISTS "state" text NOT NULL DEFAULT 'ready';

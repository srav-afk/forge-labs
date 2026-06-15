-- Create "workers" table
CREATE TABLE "public"."workers" (
  "id" text NOT NULL,
  "endpoint" text NOT NULL,
  "runtime_kind" text NOT NULL,
  "capabilities" jsonb NULL,
  "registered_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_workers_runtime_kind" to table: "workers"
CREATE INDEX "idx_workers_runtime_kind" ON "public"."workers" ("runtime_kind");
-- Create "servable_models" table
CREATE TABLE "public"."servable_models" (
  "id" bigserial NOT NULL,
  "worker_id" text NOT NULL,
  "base_model" text NOT NULL,
  "adapter" text NULL,
  "max_context" bigint NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_workers_models" FOREIGN KEY ("worker_id") REFERENCES "public"."workers" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_servable_models_adapter" to table: "servable_models"
CREATE INDEX "idx_servable_models_adapter" ON "public"."servable_models" ("adapter");
-- Create index "idx_servable_models_base_model" to table: "servable_models"
CREATE INDEX "idx_servable_models_base_model" ON "public"."servable_models" ("base_model");
-- Create index "idx_servable_models_worker_id" to table: "servable_models"
CREATE INDEX "idx_servable_models_worker_id" ON "public"."servable_models" ("worker_id");

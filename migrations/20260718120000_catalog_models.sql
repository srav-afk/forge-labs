-- Create "catalog_models" table
CREATE TABLE "public"."catalog_models" (
  "id" text NOT NULL,
  "name" text NOT NULL,
  "base_model" text NOT NULL,
  "adapter" text NULL,
  "created_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX "idx_catalog_models_name" ON "public"."catalog_models" ("name");
CREATE INDEX "idx_catalog_models_base_model" ON "public"."catalog_models" ("base_model");
CREATE INDEX "idx_catalog_models_adapter" ON "public"."catalog_models" ("adapter");
CREATE UNIQUE INDEX "catalog_models_identity_uniq" ON "public"."catalog_models" ("base_model", COALESCE("adapter", ''));

-- Create "catalog_model_assignments" table
CREATE TABLE "public"."catalog_model_assignments" (
  "id" text NOT NULL,
  "model_id" text NOT NULL,
  "worker_id" text NOT NULL,
  "created_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_catalog_assignments_model" FOREIGN KEY ("model_id") REFERENCES "public"."catalog_models" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
CREATE INDEX "idx_catalog_model_assignments_model_id" ON "public"."catalog_model_assignments" ("model_id");
CREATE INDEX "idx_catalog_model_assignments_worker_id" ON "public"."catalog_model_assignments" ("worker_id");
CREATE UNIQUE INDEX "catalog_assignments_uniq" ON "public"."catalog_model_assignments" ("model_id", "worker_id");

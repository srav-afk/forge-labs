CREATE TABLE "public"."planner_objectives" (
  "id" bigserial NOT NULL,
  "target_ttft_ms" double precision NULL,
  "target_tpot_ms" double precision NULL,
  "max_cost_per_hour" double precision NULL,
  "weight_load" double precision NULL,
  "weight_latency" double precision NULL,
  "weight_cost" double precision NULL,
  "weight_affinity" double precision NULL,
  "eval_interval_sec" bigint NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE "public"."planner_decisions" (
  "id" bigserial NOT NULL,
  "version" bigint NOT NULL,
  "objective_hash" text NOT NULL,
  "policy_json" jsonb NULL,
  "winning_score" double precision NULL,
  "reason" text NULL,
  "created_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX "idx_planner_decisions_version" ON "public"."planner_decisions" ("version");

data "external_schema" "gorm" {
  program = ["go", "run", "./tools/atlasloader"]
}

env "gorm" {
  src = data.external_schema.gorm.url
  dev = "docker://postgres/17/dev"
  url = getenv("FORGE_DB_URL")
  migration {
    dir = "file://migrations"
  }
}

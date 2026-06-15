package main

import (
	"fmt"
	"io"
	"os"

	"ariga.io/atlas-provider-gorm/gormschema"

	"github.com/srav-afk/forge-labs/services/registry/models"
)

func main() {
	stmts, err := gormschema.New("postgres").Load(
		&models.Worker{},
		&models.ServableModel{},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load gorm schema: %v\n", err)
		os.Exit(1)
	}
	io.WriteString(os.Stdout, stmts)
}

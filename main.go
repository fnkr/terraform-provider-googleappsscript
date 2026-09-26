package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/fnkr/terraform-provider-googleappsscript/internal/provider"
)

//go:generate terraform fmt -recursive ./examples/
//go:generate go tool tfplugindocs generate --provider-name googleappsscript

// Set by goreleaser.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/fnkr/googleappsscript",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/radius-project/radius/terraform-provider/radius"
)

var (
	version string = "dev"
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "hashicorp.com/microsoft/radius",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), radius.New(version), opts)

	if err != nil {
		log.Fatal(err.Error())
	}
}

package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	log.Println(fmt.Sprintf("Server running at http://localhost:%s", port))
	log.Println(fmt.Sprintf("Check http://localhost:%s/healthz for status", port))
	crt := os.Getenv("CERT")
	key := os.Getenv("KEY")

	if crt == "" {
		err := startMagpieServer()
	} else {
		err := startSecureMagpieServer([]byte(crt), []byte(key))
	}
	if err != nil {
		log.Println("Terminating Magpie")
		os.Exit(1)
	}
}

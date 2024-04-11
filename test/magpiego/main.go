package main

import (
	"log"
	"os"
	"strings"
)

func main() {
	log.Printf("Server running at http://localhost:%s\n", port)
	log.Printf("Check http://localhost:%s/healthz for status\n", port)

	crt := strings.TrimSpace(os.Getenv("TLS_CERT"))
	key := strings.TrimSpace(os.Getenv("TLS_KEY"))
	var err error
	if crt == "" && key == "" {
		log.Println("Starting magpie in http mode")
		err = startHTTPServer()
	} else {
		log.Println("Starting magpie in https mode")
		err = startHTTPSServer([]byte(crt), []byte(key))
	}
	if err != nil {
		log.Println("Terminating Magpie. Encountered error - ", err.Error())
		os.Exit(1) //nolint:forbidigo // this is OK inside the main function.
	}
}

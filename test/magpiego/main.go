package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	log.Println(fmt.Sprintf("Server running at http://localhost:%s", port))
	log.Println(fmt.Sprintf("Check http://localhost:%s/healthz for status", port))

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
		os.Exit(1)
	}
}

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

	crt := strings.TrimSpace(os.Getenv("CERT"))
	key := strings.TrimSpace(os.Getenv("KEY"))
	var err error
	if crt == "" && key == "" {
		log.Println("Starting magpie in http mode")
		err = startMagpieServer()
	} else if crt == "" || key == "" {
		log.Println("certificate or key is not provided. Starting magpie in http mode")
		err = startMagpieServer()
	} else {
		log.Println("Starting magpie in https mode")
		err = startSecureMagpieServer([]byte(crt), []byte(key))
	}
	if err != nil {
		log.Println("Terminating Magpie. Encountered error - ", err.Error())
		os.Exit(1)
	}
}

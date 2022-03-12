package main

import (
	"fmt"
	"log"
)

func main() {
	log.Println(fmt.Sprintf("Server running at http://localhost:%s", port))
	log.Println(fmt.Sprintf("Check http://localhost:%s/healthz for status", port))
	startMagpieServer()
}

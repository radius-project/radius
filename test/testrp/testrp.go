package main

import (
	"fmt"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/", HelloServer)
	http.ListenAndServe(":5000", nil)
}

func HelloServer(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received request with URL: %s\n", r.URL.Path)
	testRPURL := "http://testrp.radius-system:5000"
	var headerName string
	var headerValue string
	queryParams := r.URL.Query()

	for k, v := range queryParams {
		if (strings.EqualFold(k, "Azure-Asyncoperation") || strings.EqualFold(k, "Location")) && len(v) > 0 {
			headerName = strings.ToLower(k)
			headerValue = testRPURL + "/" + v[0]
			w.Header().Set(headerName, headerValue)
			fmt.Printf("Added %s header with value: %s\n", headerName, headerValue)
		}
	}
	fmt.Fprintf(w, "Hello, %s!", r.URL.Path[1:])
}

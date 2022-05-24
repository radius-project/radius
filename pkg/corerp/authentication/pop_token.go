package authentication

import (
	"fmt"
	"net/http"
	"strings"
)

type POPToken struct {
	token interface{}
}

func RetrievePOPToken(req *http.Request) {
	fmt.Println("retrieve token from header")
	auth := strings.TrimSpace(req.Header.Get("Authorization"))
	if auth == "" {
		fmt.Println("Authorization header is empty. Bad request")
	}
	parts := strings.Split(auth, " ")
	if len(parts) < 2 || strings.ToLower(parts[0]) != "bearer" {
		fmt.Println("Bearer token not found. Bad request")
	}

	token := parts[1]

	// Empty bearer tokens aren't valid
	if len(token) == 0 {
		fmt.Println("Bearer token not found. Bad request")
	}

	fmt.Println(fmt.Sprintf("POP token retrieved is - %v", token))
}

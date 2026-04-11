package main

import (
	"fmt"
	"os"
	"time"

	"github.com/conduit-obs/conduit/internal/auth"
)

func main() {
	keyPath := os.Args[1]
	tenantID := os.Args[2]

	privKey, err := auth.LoadRSAPrivateKey(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	token, err := auth.IssueToken(privKey, "conduit", "conduit-api", "dev-user", tenantID, []string{"admin"}, 24*time.Hour)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(token)
}

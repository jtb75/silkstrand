//go:build ignore

// Standalone helper to generate bcrypt hashes for seed data.
// Run from the backoffice module directory (has golang.org/x/crypto):
//
//	cd backoffice && go run ../scripts/hash-password.go [password]
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	password := "admin123"
	if len(os.Args) > 1 {
		password = os.Args[1]
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(hash))
}

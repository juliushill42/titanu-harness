package main

import (
	"fmt"
	"github.com/juliushill42/titanu-harness/internal/harness"
	"log"
	"os"
)

func main() {
	root := os.Getenv("TITANU_HARNESS_ROOT")
	if root == "" {
		root = "."
	}
	addr := os.Getenv("TITANU_HARNESS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8844"
	}
	a, e := harness.New(root, os.Getenv("TITANU_MODEL_ENDPOINT"), os.Getenv("TITANU_MODEL_ID"))
	if e != nil {
		log.Fatal(e)
	}
	fmt.Printf("TitanU Harness listening at http://%s\nLogin token (file %s): %s\n", addr, a.TokenPath(), a.TokenHex())
	log.Fatal(a.Serve(addr))
}

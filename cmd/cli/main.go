package main

import (
	"log"

	"github.com/naoya0117/shuron2025/api/cmd/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
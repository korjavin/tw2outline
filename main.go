package main

import (
	"log"

	"github.com/korjavin/tw2outline/internal/app"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Println("Starting Twitter to Outline bot")

	application, err := app.New()
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	application.Run()
}

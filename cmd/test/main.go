package main

import (
	"fmt"
	"log"

	"github.com/CaptainFallaway/Allower/internal/config"
)

func main() {
	conf, err := config.Load("./config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("Config loaded: %+v\n", conf)
}

package main

import (
	"fmt"
	"log"

	"github.com/mohammadhprp/throttle/internal/config"
)

func main() {
	config.LoadDotEnv()

	cfg := config.Load()

	address := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("listening on %s", address)
}

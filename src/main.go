package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"personal/discord_go/src/client"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	token := os.Getenv("DISCORD_TOKEN")
	defaultPrefix := os.Getenv("DEFAULT_PREFIX")

	bot, err := client.NewBot(token, defaultPrefix)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal, shutting down gracefully...")
		cancel()
	}()

	if err := bot.ConnectToGateway(ctx); err != nil {
		log.Fatalf("Failed to connect to gateway: %v", err)
	}
}

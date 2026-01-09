package main

import (
	"log"
	"os"

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
	bot := client.NewBot(token, defaultPrefix)

	bot.ConnectToGateway()
}

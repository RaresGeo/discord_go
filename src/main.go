package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

const DISCORD_API = "https://discordapp.com/api/v6"

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	bot := client.newBot(os.Getenv("DISCORD_TOKEN"))

	bot.connectToGateway()
}

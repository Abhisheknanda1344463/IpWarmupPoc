package main

import (
	"log"
	"net/http"
	"os"

	"domain-vetting-poc/ai"
	"domain-vetting-poc/vetting"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	// Get port from environment (for cloud deployment) or default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Vetting endpoints
	http.HandleFunc("/vet", vetting.VetHandler)
	http.HandleFunc("/warmup", vetting.WarmupHandler)

	// AI Chat endpoints (Backend-Driven)
	http.HandleFunc("/chat/start", ai.StartChatHandler) // Initialize new chat session
	http.HandleFunc("/chat", ai.ChatHandler)            // Send message to chat

	// Static files
	http.HandleFunc("/", vetting.IndexHandler)

	log.Printf("✅ warmup-vet service listening on :%s\n", port)
	log.Println("📍 Endpoints:")
	log.Println("   POST /vet          - Domain vetting")
	log.Println("   POST /warmup       - Warmup calculation")
	log.Println("   POST /chat/start   - Start AI chat session")
	log.Println("   POST /chat         - Send chat message")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

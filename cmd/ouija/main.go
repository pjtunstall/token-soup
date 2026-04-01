package main

import (
	"flag"
	"log"
	"net/http"
	"ouija/internal/chat"

	"github.com/joho/godotenv"
)

var addr = flag.String("addr", ":8080", "http service address")

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: could not load .env file: %v", err)
	}

	flag.Parse()
	hub := chat.NewHub()
	ai := chat.NewAiClient(hub)
	go hub.Run()
	go ai.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		chat.ServeWs(hub, w, r)
	})
	err := http.ListenAndServe(*addr, mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

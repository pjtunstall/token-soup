package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"
	"token-soup/internal/chat"

	"github.com/joho/godotenv"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

var addr = flag.String("addr", ":8080", "http service address")

func initKeyCache(url string) *jwk.Cache {
	ctx := context.Background()
	cache := jwk.NewCache(ctx)
	cache.Register(url, jwk.WithMinRefreshInterval(15*time.Minute))
	return cache
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: could not load .env file: %v", err)
	}

	jwksURL := os.Getenv("SUPABASE_JWKS_URL")
	if jwksURL == "" {
		log.Fatal("SUPABASE_JWKS_URL is required")
	}
	cache := initKeyCache(jwksURL)

	flag.Parse()
	store := &chat.HistoryStore{}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", chat.ChatHandler(store, cache, jwksURL))

	handler := chat.WithCORS(mux)
	err := http.ListenAndServe(*addr, handler)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

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
	"github.com/lestrrat-go/jwx/v2/jwt"
)

var addr = flag.String("addr", ":8080", "http service address")

func initKeyCache(url string) *jwk.Cache {
	ctx := context.Background()
	cache := jwk.NewCache(ctx)
	
	// Fetch once and refresh every 15 mins (or if verification fails).
	cache.Register(url, jwk.WithMinRefreshInterval(15*time.Minute))
	
	return cache
}

// verifyToken uses the JWKS registered on cache (same URL as initKeyCache).
func verifyToken(ctx context.Context, cache *jwk.Cache, jwksURL, tokenStr string) (jwt.Token, error) {
	set, err := cache.Get(ctx, jwksURL)
	if err != nil {
		return nil, err
	}
	return jwt.Parse([]byte(tokenStr), jwt.WithKeySet(set))
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
	hub := chat.NewHub(cache, jwksURL)
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

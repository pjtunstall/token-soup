package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"token-soup/internal/chat"

	"github.com/jackc/pgx/v5/pgxpool"
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

	var persist *chat.Persist
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		pool, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			log.Fatalf("DATABASE_URL: %v", err)
		}
		defer pool.Close()
		persist = &chat.Persist{Pool: pool}
	} else {
		log.Print("warning: DATABASE_URL unset; chats are not persisted to Postgres")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", chat.ChatHandler(store, persist, cache, jwksURL))
	rolesColl := chat.RolesCollectionHandler(persist, cache, jwksURL)
	mux.HandleFunc("GET /api/roles", rolesColl)
	mux.HandleFunc("POST /api/roles", rolesColl)
	rolesItem := chat.RolesItemHandler(persist, cache, jwksURL)
	mux.HandleFunc("PATCH /api/roles/{id}", rolesItem)
	mux.HandleFunc("DELETE /api/roles/{id}", rolesItem)

	mux.HandleFunc("GET /api/chats", chat.ChatsCollectionHandler(persist, cache, jwksURL))
	mux.HandleFunc("GET /api/chats/{id}", chat.ChatsItemHandler(persist, cache, jwksURL))

	handler := chat.WithCORS(mux)
	err := http.ListenAndServe(*addr, handler)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

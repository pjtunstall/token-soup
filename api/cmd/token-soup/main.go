package main

import (
	"context"
	"flag"
	"fmt"
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

var (
	addr = flag.String("addr", ":8080", "http service address")
	mode = flag.String("mode", "local", "auth/storage mode: local or supabase")
)

func initKeyCache(url string) *jwk.Cache {
	ctx := context.Background()
	cache := jwk.NewCache(ctx)
	cache.Register(url, jwk.WithMinRefreshInterval(15*time.Minute))
	return cache
}

func newAuthenticator(mode chat.AuthMode) (*chat.Authenticator, error) {
	switch mode {
	case chat.AuthModeLocal, chat.AuthModeSupabase:
	default:
		return nil, fmt.Errorf("invalid -mode %q (want local or supabase)", mode)
	}

	auth := &chat.Authenticator{Mode: mode}
	if mode == chat.AuthModeSupabase {
		jwksURL := os.Getenv("SUPABASE_JWKS_URL")
		if jwksURL == "" {
			return nil, fmt.Errorf("SUPABASE_JWKS_URL is required in supabase mode")
		}
		auth.JWKSURL = jwksURL
		auth.Cache = initKeyCache(jwksURL)
		return auth, nil
	}

	log.Print("mode=local: accepting Bearer local-dev (no Supabase JWKS)")
	return auth, nil
}

func openPersist(mode chat.AuthMode, dsn string) (*chat.Persist, func(), error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		if mode == chat.AuthModeLocal {
			return nil, nil, fmt.Errorf("DATABASE_URL is required in local mode")
		}
		log.Print("warning: DATABASE_URL unset; chats are not persisted to Postgres")
		return nil, func() {}, nil
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("DATABASE_URL: %w", err)
	}
	return &chat.Persist{Pool: pool}, pool.Close, nil
}

func newMux(store *chat.HistoryStore, persist *chat.Persist, auth *chat.Authenticator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", chat.ChatHandler(store, persist, auth))
	rolesColl := chat.RolesCollectionHandler(persist, auth)
	mux.HandleFunc("GET /api/roles", rolesColl)
	mux.HandleFunc("POST /api/roles", rolesColl)
	rolesItem := chat.RolesItemHandler(persist, auth)
	mux.HandleFunc("PATCH /api/roles/{id}", rolesItem)
	mux.HandleFunc("DELETE /api/roles/{id}", rolesItem)
	mux.HandleFunc("GET /api/chats", chat.ChatsCollectionHandler(persist, auth))
	mux.HandleFunc("GET /api/chats/{id}", chat.ChatsItemHandler(persist, auth))
	return chat.WithCORS(mux)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: could not load .env file: %v", err)
	}

	flag.Parse()

	auth, err := newAuthenticator(chat.AuthMode(strings.TrimSpace(*mode)))
	if err != nil {
		log.Fatal(err)
	}

	persist, closePersist, err := openPersist(auth.Mode, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer closePersist()

	handler := newMux(&chat.HistoryStore{}, persist, auth)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

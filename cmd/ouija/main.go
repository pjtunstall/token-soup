package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"ouija/internal/chat"
	"path/filepath"
)

var addr = flag.String("addr", ":8080", "http service address")

// homeHTMLPath resolves home.html relative to the process working directory.
// go run ./cmd/ouija is usually started from the repo root; a binary may be
// run from cmd/ouija instead — try common layouts before ServeFile.
func homeHTMLPath() string {
	for _, p := range []string{
		"home.html",
		filepath.Join("..", "home.html"),
		filepath.Join("..", "..", "home.html"),
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return "home.html"
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	http.ServeFile(w, r, homeHTMLPath())
}

func main() {
	flag.Parse()
	hub := chat.NewHub()
	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serveHome)
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		chat.ServeWs(hub, w, r)
	})
	err := http.ListenAndServe(*addr, mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
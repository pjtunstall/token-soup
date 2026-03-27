package main

import (
	"flag"
	"log"
	"net/http"
	"ouija/internal/chat"
)

var addr = flag.String("addr", ":8080", "http service address")

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, "../../home.html")
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
package main

import (
	"log"
	"net/http"

	"game-realtime-gm/backend/internal/router"
)

func main() {
	r := router.New()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	log.Println("server listening on :8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

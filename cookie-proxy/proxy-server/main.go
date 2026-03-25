package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	Version     = "1.0.0"
	Addr        = "127.0.0.1:7070"
	IdleTimeout = 30 * time.Minute
)

func main() {
	log.SetPrefix("[proxy-server] ")
	log.Printf("Cookie Proxy v%s starting on %s", Version, Addr)

	hub := NewHub()
	srv := NewServer(hub)

	httpSrv := &http.Server{
		Addr:    Addr,
		Handler: srv,
	}

	// Idle shutdown timer — reset on each request
	idleTimer := time.AfterFunc(IdleTimeout, func() {
		log.Println("idle timeout reached, shutting down")
		os.Exit(0)
	})
	srv.OnActivity = func() { idleTimer.Reset(IdleTimeout) }

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("shutting down")
		os.Exit(0)
	}()

	log.Printf("listening on %s", Addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen error: %v", err)
	}
}

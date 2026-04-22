package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/babelsuite/babelsuite/internal/envloader"
)

func main() {
	envloader.Load(".env", "../.env")

	controlPlane, err := newControlPlane(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	addr := envOr("PORT", "8090")
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           controlPlane.handler,
		ReadHeaderTimeout: durationOr("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       durationOr("HTTP_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:      durationOr("HTTP_WRITE_TIMEOUT", 2*time.Minute),
		IdleTimeout:       durationOr("HTTP_IDLE_TIMEOUT", 2*time.Minute),
		MaxHeaderBytes:    1 << 14, // 16 KB
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	go func() {
		<-stop
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()

	log.Printf("babelsuite server listening on %s using %s", addr, controlPlane.dbDriver)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := controlPlane.Close(shutdownCtx); err != nil {
		log.Printf("control plane shutdown: %v", err)
	}
}

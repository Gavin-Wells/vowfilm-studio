package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"vowfilm/server/internal/config"
	"vowfilm/server/internal/studio"
)

func val(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func main() {
	if err := config.LoadEnv(val("VOWFILM_ENV_FILE", ".env")); err != nil {
		log.Fatal(err)
	}
	data, _ := filepath.Abs(val("VOWFILM_DATA_DIR", "data"))
	concurrency, _ := strconv.Atoi(val("VOWFILM_CONCURRENCY", "2"))
	if concurrency < 1 || concurrency > 6 {
		concurrency = 2
	}
	cfg := studio.Config{
		DatabaseDriver: val("DATABASE_DRIVER", "sqlite"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		SetupToken:     os.Getenv("VOWFILM_SETUP_TOKEN"),
		PublicMediaURL: os.Getenv("PUBLIC_MEDIA_BASE_URL"),
		DataDir:        data,
		BaseURL:        val("STARNET_BASE_URL", val("OPENAI_BASE_URL", "https://open.embervale.cn")),
		APIKey:         val("STARNET_API_KEY", os.Getenv("OPENAI_API_KEY")),
		LLMModel:       val("STARNET_LLM_MODEL", val("OPENAI_MODEL", "openai/gpt-6-astra")),
		VideoModel:     val("STARNET_VIDEO_MODEL", "volcengine/doubao-seedance-2-0-mini-260615"),
		Token:          os.Getenv("GO_BACKEND_TOKEN"),
		Addr:           val("VOWFILM_ADDR", "127.0.0.1:8097"),
		Concurrency:    concurrency,
	}
	if len(cfg.Token) < 24 {
		log.Fatal("GO_BACKEND_TOKEN must contain at least 24 characters")
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	app, err := studio.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: app.Handler(), ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		app.Shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	log.Printf("Vowfilm Go studio listening on %s", cfg.Addr)
	if err = srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

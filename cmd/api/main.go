package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/payment-processor-rinha/internal/api"
	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment/processors"
	worker "github.com/payment-processor-rinha/internal/worker"
	"github.com/redis/go-redis/v9"
)

const redisAddr = "redis:6379"

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
		Protocol: 2,
	})
	defer redisClient.Close()

	concurrency, err := strconv.Atoi(getEnv("CONCURRENCY", "30"))
	if err != nil {
		panic(err)
	}

	blockCh := make(chan error, 2)
	queue := make(chan []byte, 10000)
	pp := paymentProcessor.NewPaymentProcessor(redisClient)
	wp := worker.NewWorker(pp, queue, concurrency)

	wp.StartWorker()

	httpServer := api.Setup(pp, queue)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil {
			blockCh <- fmt.Errorf("failed to run server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	close(queue)
	log.Println("shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("http server shutdown failed: %v", err)
	}
	log.Println("server exiting.")
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return defaultValue
	}
	return value
}

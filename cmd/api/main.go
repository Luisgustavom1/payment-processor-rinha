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
	worker "github.com/payment-processor-rinha/internal/application/payment/workers"
	"github.com/redis/go-redis/v9"
)

const redisAddr = "redis:6379"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
		Protocol: 3,

		PoolSize:        50,
		PoolTimeout:     10 * time.Second,
		MinIdleConns:    10,
		MaxIdleConns:    20,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,

		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,

		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	})
	defer redisClient.Close()

	concurrency, err := strconv.Atoi(getEnv("CONCURRENCY", "20"))
	if err != nil {
		panic(err)
	}

	master, err := strconv.ParseBool(getEnv("MASTER", "false"))
	if err != nil {
		panic(err)
	}

	blockCh := make(chan error, 2)
	queue := make(chan []byte, 10000)
	pp := paymentProcessor.NewPaymentProcessor(ctx, redisClient)

	pw := worker.NewPaymentWorker(pp, queue, concurrency)
	pw.StartPaymentWorker()

	hcw := worker.NewHealthCheckPool(pp)
	hcw.StartHealthCheckWorker(master)

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

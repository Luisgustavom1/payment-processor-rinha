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

	"github.com/hibiken/asynq"
	"github.com/payment-processor-rinha/internal/api"
	paymentProcessor "github.com/payment-processor-rinha/internal/application/payment"
	paymentTask "github.com/payment-processor-rinha/internal/application/payment/tasks"
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

	concurrency, err := strconv.Atoi(getEnv("CONCURRENCY", "20"))
	if err != nil {
		panic(err)
	}

	srv := asynq.NewServerFromRedisClient(
		redisClient,
		asynq.Config{
			Concurrency:     concurrency,
			ShutdownTimeout: 30 * time.Second,
		},
	)

	asynqClient := asynq.NewClientFromRedisClient(redisClient)
	defer asynqClient.Close()

	pp := paymentProcessor.NewPaymentProcessor(redisClient)
	mux := asynq.NewServeMux()
	mux.Handle(paymentTask.ProcessPayment, pp)

	blockCh := make(chan error, 2)

	httpServer := api.Setup(pp, asynqClient)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil {
			blockCh <- fmt.Errorf("failed to run server: %v", err)
		}
	}()

	go func() {
		fmt.Println("starting asynq server...")
		if err := srv.Run(mux); err != nil {
			blockCh <- fmt.Errorf("could not run server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down servers...")

	srv.Shutdown()
	log.Println("asynq server shut down.")

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

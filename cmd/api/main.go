package main

import (
	"log"

	"github.com/payment-processor-rinha/internal/api"
	"github.com/redis/go-redis/v9"
)

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})

	log.Fatal(api.Setup(redisClient))
}

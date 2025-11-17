package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/segmentio/kafka-go"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

type Event struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Time string `json:"time"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	KafkaOK bool   `json:"kafka_ok"`
}

func main() {
	port := getEnv("PORT", "8082")
	broker := getEnv("KAFKA_BROKER", "kafka:9092")

	topics := map[string]string{
		"user":    "events.user",
		"payment": "events.payment",
		"movie":   "events.movie",
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/events/health", func(w http.ResponseWriter, r *http.Request) {
		kafkaOK := checkKafka(broker)

		resp := HealthResponse{
			Status:  "ok",
			KafkaOK: kafkaOK,
		}

		status := http.StatusOK
		if !kafkaOK {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	})

	makeHandler := func(eventType string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			event := Event{
				Type: eventType,
				Data: fmt.Sprintf("%s event happened", eventType),
				Time: time.Now().Format(time.RFC3339),
			}

			body, _ := json.Marshal(event)

			writer := &kafka.Writer{
				Addr:     kafka.TCP(broker),
				Topic:    topics[eventType],
				Balancer: &kafka.LeastBytes{},
			}
			defer writer.Close()

			if err := writer.WriteMessages(context.Background(),
				kafka.Message{Value: body},
			); err != nil {
				http.Error(w, "failed to send kafka message", 500)
				return
			}

			log.Printf("[producer] event=%s → topic=%s", eventType, topics[eventType])
			w.WriteHeader(http.StatusCreated)
		}
	}

	mux.HandleFunc("/api/events/user", makeHandler("user"))
	mux.HandleFunc("/api/events/payment", makeHandler("payment"))
	mux.HandleFunc("/api/events/movie", makeHandler("movie"))

	log.Printf("[events-service] started on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func checkKafka(broker string) bool {
	conn, err := net.DialTimeout("tcp", broker, 2*time.Second)
	if err != nil {
		log.Printf("[health] Kafka unreachable: %v", err)
		return false
	}
	_ = conn.Close()
	return true
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

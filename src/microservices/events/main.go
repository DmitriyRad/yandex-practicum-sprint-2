package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/segmentio/kafka-go"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type Event struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Time string `json:"time"`
}

type HealthResponse struct {
	Status bool `json:"status"`
}

type SuccessResponse struct {
	Status string `json:"status"`
}

func main() {
	port := getEnv("PORT", "8082")
	broker := getEnv("KAFKA_BROKERS", "kafka:9092")
	topic := "cinema.events"

	writer := &kafka.Writer{
		Addr:     kafka.TCP(broker),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	defer writer.Close()

	// Запуск consumer в фоне
	go consumeLoop(broker, topic)

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/api/events/health", func(w http.ResponseWriter, r *http.Request) {
		resp := HealthResponse{Status: true}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Универсальный обработчик для событий
	handleEvent := func(eventType string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			event := Event{
				Type: eventType,
				Data: fmt.Sprintf("Event from %s endpoint", eventType),
				Time: time.Now().Format(time.RFC3339),
			}

			eventJSON, _ := json.Marshal(event)
			err := writer.WriteMessages(context.Background(), kafka.Message{
				Value: eventJSON,
			})
			if err != nil {
				http.Error(w, "failed to produce message: "+err.Error(), 500)
				return
			}

			log.Printf("[producer] sent %s event: %s", eventType, eventJSON)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(SuccessResponse{Status: "success"})
		}
	}

	// Регистрируем обработчики
	mux.HandleFunc("/api/events/movie", handleEvent("movie"))
	mux.HandleFunc("/api/events/user", handleEvent("user"))
	mux.HandleFunc("/api/events/payment", handleEvent("payment"))

	log.Printf("[events-service] started on port %s", port)
	server := &http.Server{Addr: ":" + port, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Ожидание сигнала остановки
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println("[events-service] shutting down...")
	_ = server.Shutdown(context.Background())
}

func consumeLoop(broker, topic string) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		Topic:    topic,
		GroupID:  "events-consumer",
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})
	defer reader.Close()

	log.Printf("[consumer] listening on topic '%s'...", topic)

	for {
		msg, err := reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("[consumer] error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var e Event
		if err := json.Unmarshal(msg.Value, &e); err == nil {
			log.Printf("[consumer] received event: type=%s data=%s time=%s", e.Type, e.Data, e.Time)
		} else {
			log.Printf("[consumer] raw message: %s", msg.Value)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

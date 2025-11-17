package main

import (
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	monolithURL            string
	moviesServiceURL       string
	eventsServiceURL       string
	gradualMigration       bool
	moviesMigrationPercent int
)

func main() {
	rand.Seed(time.Now().UnixNano())
	port := getEnv("PORT", "8000")
	monolithURL = getEnv("MONOLITH_URL", "http://monolith:9080")
	moviesServiceURL = getEnv("MOVIES_SERVICE_URL", "http://movies-service:8081")
	eventsServiceURL = getEnv("EVENTS_SERVICE_URL", "http://events-service:8082")

	gradualMigration = getEnv("GRADUAL_MIGRATION", "false") == "true"
	moviesMigrationPercent, _ = strconv.Atoi(getEnv("MOVIES_MIGRATION_PERCENT", "0"))

	http.HandleFunc("/", proxyHandler)

	log.Printf("[proxy] Service started on port %s", port)
	log.Printf("[proxy] Monolith URL: %s", monolithURL)
	log.Printf("[proxy] Movies URL: %s", moviesServiceURL)
	log.Printf("[proxy] Events URL: %s", eventsServiceURL)
	log.Printf("[proxy] Gradual migration: %v (%d%%)", gradualMigration, moviesMigrationPercent)

	http.HandleFunc("/api/proxy/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("[proxy] failed to start: %v", err)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	target := routeRequest(r)

	reqURL := target + r.URL.Path
	log.Printf("[proxy] → %s %s (target: %s)", r.Method, r.URL.Path, reqURL)

	req, err := http.NewRequest(r.Method, reqURL, r.Body)
	if err != nil {
		http.Error(w, "cannot create request", http.StatusInternalServerError)
		return
	}

	req.Header = r.Header.Clone()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[proxy] error forwarding to %s: %v", reqURL, err)
		http.Error(w, "target unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	duration := time.Since(start)
	log.Printf("[proxy] ← %d %s (%s)", resp.StatusCode, r.URL.Path, duration)
}

func routeRequest(r *http.Request) string {
	path := strings.ToLower(r.URL.Path)

	// Если включен фиче-флаг — направляем часть трафика на новый movies-service
	if gradualMigration && (strings.HasPrefix(path, "/movies") || strings.HasPrefix(path, "/api/movies")) {
		if rand.Intn(100) < moviesMigrationPercent {
			return moviesServiceURL
		}
		return monolithURL
	}

	// Пример маршрута на events-service
	if strings.HasPrefix(path, "/events") {
		return eventsServiceURL
	}

	// По умолчанию всё в монолит
	return monolithURL
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

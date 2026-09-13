package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type server struct {
	infrai *InfraiClient
	mu     sync.Mutex
	stages map[string]OrderStage
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	s := &server{infrai: NewInfraiClient(key), stages: make(map[string]OrderStage)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/{orderID}/room", s.createRoom)
	mux.HandleFunc("POST /orders/{orderID}/chat-token", s.chatToken)
	mux.HandleFunc("POST /orders/{orderID}/updates", s.publishUpdate)
	log.Printf("order room service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (s *server) createRoom(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderID")
	err := s.infrai.CreateChannel(r.Context(), orderChannel(orderID), "room:"+orderID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"channel": orderChannel(orderID)})
}

func (s *server) chatToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClientID string `json:"client_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ClientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}
	data, err := s.infrai.IssueToken(r.Context(), input.ClientID, orderChannel(r.PathValue("orderID")))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *server) publishUpdate(w http.ResponseWriter, r *http.Request) {
	var update OrderUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	update.OrderID = r.PathValue("orderID")
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if strings.TrimSpace(idempotencyKey) == "" {
		http.Error(w, "Idempotency-Key is required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateTransition(s.stages[update.OrderID], update); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := s.infrai.Publish(r.Context(), update, idempotencyKey); err != nil {
		writeError(w, err)
		return
	}
	s.stages[update.OrderID] = update.Stage
	writeJSON(w, http.StatusAccepted, update)
}

func writeError(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		http.Error(w, apiErr.Message, apiErr.HTTPStatus)
		return
	}
	http.Error(w, "upstream request failed", http.StatusBadGateway)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/Shervina-lim/multi-agent-orchestrator/internal/manager"
	"github.com/Shervina-lim/multi-agent-orchestrator/internal/rmq"
	"github.com/google/uuid"
)

func main() {
	rmqURL := os.Getenv("RABBITMQ_URL")
	if rmqURL == "" {
		rmqURL = "amqp://guest:guest@localhost:5672/"
	}

	conn, err := rmq.Dial(rmqURL)
	if err != nil {
		log.Fatalf("rmq connect: %v", err)
	}
	defer conn.Close()

	// shard configuration (for sharded manager deployments)
	shardID := 0
	shardCount := 1
	if v := os.Getenv("SHARD_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			shardID = n
		}
	}
	if v := os.Getenv("SHARD_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			shardCount = n
		}
	}

	mgr := manager.New(conn, shardID, shardCount)

	http.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Type    string                 `json:"type"`
			Payload map[string]interface{} `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := uuid.New().String()
		if err := mgr.Submit(id, req.Type, req.Payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	})

	http.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/tasks/"):]
		t := mgr.Get(id)
		if t == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	})

	log.Println("manager: listening :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

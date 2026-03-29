package main

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/example/picomanager/internal/rmq"
	"github.com/streadway/amqp"
)

func main() {
	rmqURL := os.Getenv("RABBITMQ_URL")
	if rmqURL == "" {
		rmqURL = "amqp://guest:guest@localhost:5672/"
	}
	conn, dialErr := rmq.Dial(rmqURL)
	if dialErr != nil {
		log.Fatalf("rmq: %v", dialErr)
	}
	defer conn.Close()

	// determine shard consumption: for simplicity worker will consume from all shard queues if needed
	// here we consume from the generic pattern by attempting to consume tasks.0..N-1 if SHARD_COUNT set
	shardCount := 1
	if v := os.Getenv("SHARD_COUNT"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			shardCount = n
		}
	}
	var msgs <-chan amqp.Delivery
	var err error
	// if single shard, use tasks.0
	if shardCount == 1 {
		msgs, err = conn.Consume("tasks.0")
	} else {
		// consume from each shard sequentially by starting a goroutine per shard
		for i := 0; i < shardCount; i++ {
			q := fmt.Sprintf("tasks.%d", i)
			m, e := conn.Consume(q)
			if e != nil {
				log.Printf("consume shard %d err: %v", i, e)
				continue
			}
			go func(ch <-chan amqp.Delivery) {
				for d := range ch {
					handleTask(d, conn)
				}
			}(m)
		}
		// exit main consumption initialiser; background goroutines handle deliveries
		select {}
	}
	if err != nil {
		log.Fatalf("consume: %v", err)
	}
	log.Println("worker: waiting for tasks")
	for d := range msgs {
		handleTask(d, conn)
	}
}

func handleTask(d amqp.Delivery, conn *rmq.Conn) {
	var req struct {
		ID       string                 `json:"id"`
		Type     string                 `json:"type"`
		Payload  map[string]interface{} `json:"payload"`
		Attempts int                    `json:"attempts,omitempty"`
	}
	if err := json.Unmarshal(d.Body, &req); err != nil {
		log.Printf("invalid task: %v", err)
		return
	}
	log.Printf("worker: got task id=%s type=%s attempts=%d", req.ID, req.Type, req.Attempts)
	// simulate processing
	time.Sleep(1 * time.Second)
	res := map[string]interface{}{"ok": true, "processed_at": time.Now().UTC().String()}
	body, _ := json.Marshal(map[string]interface{}{"id": req.ID, "result": res, "attempts": req.Attempts})
	// compute shard count from env (worker may be consuming multiple shards)
	shardCount := 1
	if v := os.Getenv("SHARD_COUNT"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			shardCount = n
		}
	}
	if err := conn.Publish("results."+computeShard(req.ID, shardCount), body); err != nil {
		log.Printf("publish result err: %v", err)
	}
}

func computeShard(id string, shardCount int) string {
	crc := crc32.ChecksumIEEE([]byte(id))
	idx := int(crc) % shardCount
	return strconv.Itoa(idx)
}

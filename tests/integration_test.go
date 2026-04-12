package tests

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	mgr "github.com/Shervina-lim/multi-agent-orchestrator/internal/manager"
	"github.com/Shervina-lim/multi-agent-orchestrator/internal/rmq"
	"github.com/google/uuid"
)

func rmqURL() string {
	if v := os.Getenv("RABBITMQ_URL"); v != "" {
		return v
	}
	return "amqp://guest:guest@localhost:5672/"
}

func setupManager(t *testing.T) (*mgr.Manager, *rmq.Conn) {
	conn, err := rmq.Dial(rmqURL())
	if err != nil {
		t.Fatalf("rmq dial: %v", err)
	}
	m := mgr.New(conn, 0, 1)
	return m, conn
}

func TestSuccessFlow(t *testing.T) {
	m, conn := setupManager(t)
	defer conn.Close()
	workerConn, err := rmq.Dial(rmqURL())
	if err != nil {
		t.Fatalf("worker rmq dial: %v", err)
	}
	defer workerConn.Close()

	id := uuid.New().String()

	// worker: consume tasks.0 and publish success (start before submit)
	msgs, err := workerConn.Consume("tasks.0")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	go func() {
		for d := range msgs {
			var req map[string]interface{}
			_ = json.Unmarshal(d.Body, &req)
			t.Logf("worker received task: %v", req["id"])
			body, _ := json.Marshal(map[string]interface{}{"id": req["id"], "result": map[string]interface{}{"ok": true}, "status": "done"})
			_ = workerConn.Publish("results.0", body)
			t.Logf("worker published result for: %v", req["id"])
		}
	}()

	// give consumer a moment to bind
	time.Sleep(200 * time.Millisecond)

	if err := m.Submit(id, "echo", map[string]interface{}{"msg": "hello"}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// wait for done
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		tsk := m.Get(id)
		if tsk != nil && tsk.Status == mgr.StatusDone {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("task did not complete in time")
}

func TestRetryFlow(t *testing.T) {
	m, conn := setupManager(t)
	defer conn.Close()
	workerConn, err := rmq.Dial(rmqURL())
	if err != nil {
		t.Fatalf("worker rmq dial: %v", err)
	}
	defer workerConn.Close()

	id := uuid.New().String()

	msgs, err := workerConn.Consume("tasks.0")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	go func() {
		seen := 0
		for d := range msgs {
			seen++
			var req map[string]interface{}
			_ = json.Unmarshal(d.Body, &req)
			t.Logf("worker(retry) got task %v attempt %d", req["id"], seen)
			if seen == 1 {
				// first attempt: fail
				body, _ := json.Marshal(map[string]interface{}{"id": req["id"], "error": "bad", "status": "failed", "attempts": 0})
				_ = workerConn.Publish("results.0", body)
				t.Logf("worker(retry) published failed for %v", req["id"])
				continue
			}
			// second attempt: succeed
			body, _ := json.Marshal(map[string]interface{}{"id": req["id"], "result": map[string]interface{}{"ok": true}, "status": "done"})
			_ = conn.Publish("results.0", body)
			t.Logf("worker(retry) published done for %v", req["id"])
		}
	}()

	// give consumer a moment to bind
	time.Sleep(200 * time.Millisecond)

	if err := m.Submit(id, "echo", map[string]interface{}{"msg": "retry"}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// wait for done
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		tsk := m.Get(id)
		if tsk != nil && tsk.Status == mgr.StatusDone {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("retry flow did not complete")
}

func TestFollowupFlow(t *testing.T) {
	m, conn := setupManager(t)
	defer conn.Close()
	workerConn, err := rmq.Dial(rmqURL())
	if err != nil {
		t.Fatalf("worker rmq dial: %v", err)
	}
	defer workerConn.Close()

	id := uuid.New().String()

	msgs, err := workerConn.Consume("tasks.0")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	go func() {
		seen := 0
		for d := range msgs {
			seen++
			var req map[string]interface{}
			_ = json.Unmarshal(d.Body, &req)
			t.Logf("worker(followup) got %v attempt %d", req["id"], seen)
			if seen == 1 {
				// request a followup
				body, _ := json.Marshal(map[string]interface{}{"id": req["id"], "status": "awaiting_followup", "followup": map[string]interface{}{"step": 2}})
				_ = workerConn.Publish("results.0", body)
				t.Logf("worker(followup) requested followup for %v", req["id"])
				continue
			}
			// final
			body, _ := json.Marshal(map[string]interface{}{"id": req["id"], "result": map[string]interface{}{"finished": true}, "status": "done"})
			_ = conn.Publish("results.0", body)
			t.Logf("worker(followup) published done for %v", req["id"])
		}
	}()

	// give consumer a moment to bind
	time.Sleep(200 * time.Millisecond)

	if err := m.Submit(id, "workflow", map[string]interface{}{"step": 1}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)

	for time.Now().Before(deadline) {
		tsk := m.Get(id)
		if tsk != nil && tsk.Status == mgr.StatusDone {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("followup flow did not complete")
}

func TestDLQFlow(t *testing.T) {
	m, conn := setupManager(t)
	defer conn.Close()
	workerConn, err := rmq.Dial(rmqURL())
	if err != nil {
		t.Fatalf("worker rmq dial: %v", err)
	}
	defer workerConn.Close()

	id := uuid.New().String()

	msgs, err := workerConn.Consume("tasks.0")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	go func() {
		for d := range msgs {
			var req map[string]interface{}
			_ = json.Unmarshal(d.Body, &req)
			t.Logf("worker(dlq) got %v", req["id"])
			// always fail
			body, _ := json.Marshal(map[string]interface{}{"id": req["id"], "error": "bad", "status": "failed", "attempts": 0})
			_ = workerConn.Publish("results.0", body)
			t.Logf("worker(dlq) published failed for %v", req["id"])
		}
	}()

	// give consumer a moment to bind
	time.Sleep(200 * time.Millisecond)

	if err := m.Submit(id, "bad", map[string]interface{}{"fail": true}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// wait for DLQ entry
	dlqMsgs, err := workerConn.Consume("deadletter.0")
	if err != nil {
		t.Fatalf("consume dlq: %v", err)
	}

	select {
	case d := <-dlqMsgs:
		if len(d.Body) == 0 {
			t.Fatalf("empty dlq body")
		}
	case <-time.After(40 * time.Second):
		t.Fatalf("no dlq message seen")
	}
}

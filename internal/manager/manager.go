package manager

import (
	"encoding/json"
	"hash/crc32"
	"strconv"
	"sync"
	"time"

	"github.com/Shervina-lim/multi-agent-orchestrator/internal/logging"
	"github.com/Shervina-lim/multi-agent-orchestrator/internal/rmq"
)

type TaskStatus string

const (
	StatusQueued  TaskStatus = "queued"
	StatusRunning TaskStatus = "running"
	StatusDone    TaskStatus = "done"
	StatusFailed  TaskStatus = "failed"
)

type Task struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Status    TaskStatus             `json:"status"`
	Attempts  int                    `json:"attempts,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type Manager struct {
	rmq        *rmq.Conn
	store      map[string]*Task
	mu         sync.RWMutex
	shardID    int
	shardCount int
}

const (
	DefaultMaxAttempts    = 3
	DefaultBackoffBase    = 1 // seconds
	DefaultBackoffFactor  = 2
	DefaultBackoffCap     = 30 // seconds
	DefaultAttemptTimeout = 30 * time.Second
)

func New(conn *rmq.Conn, shardID, shardCount int) *Manager {
	if shardCount <= 0 {
		shardCount = 1
	}
	m := &Manager{rmq: conn, store: make(map[string]*Task), shardID: shardID, shardCount: shardCount}
	m.startResultConsumer()
	return m
}

func (m *Manager) Submit(id, typ string, payload map[string]interface{}) error {
	t := &Task{ID: id, Type: typ, Payload: payload, Status: StatusQueued, CreatedAt: time.Now()}
	t.Attempts = 0
	m.mu.Lock()
	m.store[id] = t
	m.mu.Unlock()
	// publish to shard-specific tasks queue and start attempt timeout
	if err := m.publishToShard(id, typ, payload, t.Attempts); err != nil {
		return err
	}
	m.startAttemptTimer(id, 0)
	return nil
}

func (m *Manager) publishToShard(id, typ string, payload map[string]interface{}, attempts int) error {
	shard := m.computeShard(id)
	q := "tasks." + shard
	body, _ := json.Marshal(map[string]interface{}{"id": id, "type": typ, "payload": payload, "attempts": attempts})
	m.mu.Lock()
	if t, ok := m.store[id]; ok {
		t.Status = StatusRunning
	}
	m.mu.Unlock()
	logging.Debugf("publishing task %s to %s (attempts=%d)", id, q, attempts)
	return m.rmq.Publish(q, body)
}

func (m *Manager) startAttemptTimer(id string, attemptNum int) {
	go func() {
		timeout := DefaultAttemptTimeout
		time.Sleep(timeout)
		m.mu.Lock()
		t, ok := m.store[id]
		if !ok {
			m.mu.Unlock()
			return
		}
		// if task already done or failed, nothing to do
		if t.Status == StatusDone || t.Status == StatusFailed {
			m.mu.Unlock()
			return
		}
		// if attempts hasn't advanced, treat as timed-out and trigger retry logic
		if t.Attempts == attemptNum {
			// increment attempts and decide retry or DLQ
			if t.Attempts < DefaultMaxAttempts {
				t.Attempts++
				t.Status = StatusQueued
				payload := t.Payload
				m.mu.Unlock()
				logging.Debugf("attempt timer: task %s timed out, retrying attempt %d", id, t.Attempts)
				// apply backoff before re-publish
				backoff := time.Duration(DefaultBackoffBase) * time.Second
				for i := 1; i < t.Attempts; i++ {
					backoff = backoff * DefaultBackoffFactor
					if backoff > time.Duration(DefaultBackoffCap)*time.Second {
						backoff = time.Duration(DefaultBackoffCap) * time.Second
						break
					}
				}
				time.Sleep(backoff)
				if err := m.publishToShard(id, t.Type, payload, t.Attempts); err != nil {
					// on publish error, mark failed
					m.mu.Lock()
					t.Status = StatusFailed
					m.mu.Unlock()
					return
				}
				// start next attempt timer
				m.startAttemptTimer(id, t.Attempts)
			} else {
				// send to dead-letter queue
				shard := m.computeShard(id)
				dlq := "deadletter." + shard
				body, _ := json.Marshal(t)
				_ = m.rmq.Publish(dlq, body)
				t.Status = StatusFailed
				m.mu.Unlock()
			}
		} else {
			m.mu.Unlock()
		}
	}()
}

func (m *Manager) Get(id string) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.store[id]; ok {
		return t
	}
	return nil
}

func (m *Manager) startResultConsumer() {
	// consume results and update store
	go func() {
		// consume results for this manager shard
		q := "results." + m.computeShardPrefix()
		logging.Infof("result consumer starting for queue: %s", q)
		msgs, err := m.rmq.Consume(q)
		if err != nil {
			logging.Infof("result consumer error: %v", err)
			return
		}
		for d := range msgs {
			logging.Debugf("result consumer got raw message: %s", string(d.Body))
			var res struct {
				ID       string                 `json:"id"`
				Result   map[string]interface{} `json:"result"`
				Error    string                 `json:"error,omitempty"`
				Attempts int                    `json:"attempts,omitempty"`
				Status   string                 `json:"status,omitempty"` // done | failed | awaiting_followup
				Followup map[string]interface{} `json:"followup,omitempty"`
			}
			if err := json.Unmarshal(d.Body, &res); err != nil {
				logging.Infof("invalid result body: %v", err)
				continue
			}
			m.mu.Lock()
			t, ok := m.store[res.ID]
			if !ok {
				m.mu.Unlock()
				continue
			}
			// update attempts from worker if provided
			if res.Attempts > 0 {
				t.Attempts = res.Attempts
			}
			switch res.Status {
			case "failed":
				// follow retry path via attempts logic
				if t.Attempts < DefaultMaxAttempts {
					t.Attempts++
					t.Status = StatusQueued
					payload := t.Payload
					m.mu.Unlock()
					// backoff
					backoff := time.Duration(DefaultBackoffBase) * time.Second
					for i := 1; i < t.Attempts; i++ {
						backoff = backoff * DefaultBackoffFactor
						if backoff > time.Duration(DefaultBackoffCap)*time.Second {
							backoff = time.Duration(DefaultBackoffCap) * time.Second
							break
						}
					}
					time.Sleep(backoff)
					if err := m.publishToShard(t.ID, t.Type, payload, t.Attempts); err != nil {
						m.mu.Lock()
						t.Status = StatusFailed
						m.mu.Unlock()
						continue
					}
					m.startAttemptTimer(t.ID, t.Attempts)
					// already unlocked earlier; continue to avoid double-unlock at end of loop
					continue
				} else {
					t.Status = StatusFailed
					// publish to DLQ
					shard := m.computeShard(t.ID)
					dlq := "deadletter." + shard
					body, _ := json.Marshal(t)
					_ = m.rmq.Publish(dlq, body)
				}
			case "awaiting_followup":
				// worker asks for another iteration with followup payload
				if res.Followup != nil {
					t.Payload = res.Followup
					t.Attempts = 0
					t.Status = StatusQueued
					payload := t.Payload
					m.mu.Unlock()
					// immediate re-publish for followup
					if err := m.publishToShard(t.ID, t.Type, payload, t.Attempts); err != nil {
						m.mu.Lock()
						t.Status = StatusFailed
						m.mu.Unlock()
						continue
					}
					m.startAttemptTimer(t.ID, t.Attempts)
					// already unlocked earlier; continue to avoid double-unlock at end of loop
					continue
				} else {
					// no followup payload, mark failed
					t.Status = StatusFailed
				}
			default:
				// treat as success
				t.Status = StatusDone
				t.Result = res.Result
			}
			m.mu.Unlock()
		}
	}()
}

func (m *Manager) computeShard(id string) string {
	crc := crc32.ChecksumIEEE([]byte(id))
	idx := int(crc) % m.shardCount
	return strconv.Itoa(idx)
}

func (m *Manager) computeShardPrefix() string {
	// return queue name for this shard
	return strconv.Itoa(m.shardID)
}

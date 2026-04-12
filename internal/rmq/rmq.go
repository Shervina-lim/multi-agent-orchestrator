package rmq

import (
	"github.com/Shervina-lim/multi-agent-orchestrator/internal/logging"
	"github.com/streadway/amqp"
)

type Conn struct {
	conn *amqp.Connection
	// keep connection only; create channels per operation to avoid sharing one channel
	ch *amqp.Channel
}

func Dial(url string) (*Conn, error) {
	c, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	return &Conn{conn: c, ch: nil}, nil
}

func (c *Conn) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Conn) Publish(queue string, body []byte) error {
	// use a dedicated channel for publish to avoid interfering with consumers
	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	// ensure queue exists
	q, err := ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		logging.Infof("rmq: queue declare error for %s: %v", queue, err)
		return err
	}
	logging.Debugf("rmq: publishing to queue %s (messages=%d consumers=%d)", q.Name, q.Messages, q.Consumers)
	if err := ch.Publish("", queue, false, false, amqp.Publishing{ContentType: "application/json", Body: body}); err != nil {
		logging.Infof("rmq: publish error to %s: %v", queue, err)
		return err
	}
	return nil
}

func (c *Conn) Consume(queue string) (<-chan amqp.Delivery, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, err
	}
	// do not close ch here; consumer needs channel open for delivery lifetime
	_, err = ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		ch.Close()
		return nil, err
	}
	msgs, err := ch.Consume(queue, "", true, false, false, false, nil)
	if err != nil {
		ch.Close()
		return nil, err
	}
	return msgs, nil
}

// helper for workers
func (c *Conn) LogInfo(msg string) {
	logging.Infof("rmq: %s", msg)
}

package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"trafficd/internal/model"
)

type Producer struct {
	w *kafka.Writer
}

func NewProducer(brokers []string, topic string) (*Producer, error) {
	if len(brokers) == 0 || topic == "" {
		return nil, fmt.Errorf("kafka: brokers/topic required")
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 5 * time.Millisecond,
		Async:        false,
		RequiredAcks: kafka.RequireOne,
	}
	return &Producer{w: w}, nil
}

func (p *Producer) Close() error {
	return p.w.Close()
}

// PublishBatch 将一批记录作为单条 Kafka 消息发送（partition key 取自首条流的 hash）。
func (p *Producer) PublishBatch(ctx context.Context, records []model.PacketRecord) error {
	if len(records) == 0 {
		return nil
	}
	b, err := json.Marshal(records)
	if err != nil {
		return err
	}
	key := records[0].Flow.Bytes()
	return p.w.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: b,
	})
}

type Consumer struct {
	r *kafka.Reader
}

func NewConsumer(brokers []string, topic, group string) (*Consumer, error) {
	if len(brokers) == 0 || topic == "" || group == "" {
		return nil, fmt.Errorf("kafka: brokers/topic/group required")
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  group,
		Topic:    topic,
		MinBytes: 1e3,
		MaxBytes: 10e6,
	})
	return &Consumer{r: r}, nil
}

func (c *Consumer) Close() error {
	return c.r.Close()
}

// Run 持续消费并回调批次；单条消息内为 JSON 数组 []model.PacketRecord。
func (c *Consumer) Run(ctx context.Context, fn func(context.Context, []model.PacketRecord) error) error {
	for {
		m, err := c.r.ReadMessage(ctx)
		if err != nil {
			return err
		}
		var batch []model.PacketRecord
		if err := json.Unmarshal(m.Value, &batch); err != nil {
			continue
		}
		if err := fn(ctx, batch); err != nil {
			return err
		}
	}
}

func ParseBrokers(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

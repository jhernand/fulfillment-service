/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package kafka

import (
	"context"
	"fmt"
	"log/slog"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// Producer wraps a Kafka producer, providing a synchronous Produce method that waits for delivery confirmation.
type Producer struct {
	logger   *slog.Logger
	producer *confluentkafka.Producer
}

// Produce sends a message to Kafka and waits for delivery confirmation.
func (p *Producer) Produce(ctx context.Context, message *confluentkafka.Message) error {
	deliveryChan := make(chan confluentkafka.Event)
	err := p.producer.Produce(message, deliveryChan)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	select {
	case e := <-deliveryChan:
		m := e.(*confluentkafka.Message)
		if m.TopicPartition.Error != nil {
			return fmt.Errorf("failed to deliver message: %w", m.TopicPartition.Error)
		}
	case <-ctx.Done():
		return fmt.Errorf("message delivery timed out: %w", ctx.Err())
	}

	return nil
}

// Close flushes pending messages and shuts down the Kafka producer.
func (p *Producer) Close() {
	p.producer.Flush(5000)
	p.producer.Close()
}

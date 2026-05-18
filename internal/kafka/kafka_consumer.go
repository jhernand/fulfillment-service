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
	"log/slog"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// Consumer wraps a Kafka consumer. Token refresh for SASL/OAUTHBEARER is handled internally by librdkafka when
// the appropriate properties are configured.
type Consumer struct {
	logger   *slog.Logger
	consumer *confluentkafka.Consumer
}

// NewConsumerFromRaw creates a Consumer from an existing confluent-kafka-go consumer. This is intended for testing
// scenarios where custom configuration is needed.
func NewConsumerFromRaw(logger *slog.Logger, consumer *confluentkafka.Consumer) *Consumer {
	return &Consumer{
		logger:   logger,
		consumer: consumer,
	}
}

// Poll polls the Kafka consumer for events with the given timeout in milliseconds.
func (c *Consumer) Poll(timeoutMs int) confluentkafka.Event {
	return c.consumer.Poll(timeoutMs)
}

// Subscribe subscribes the consumer to the given topic.
func (c *Consumer) Subscribe(topic string) error {
	return c.consumer.Subscribe(topic, nil)
}

// Close closes the Kafka consumer.
func (c *Consumer) Close() {
	c.consumer.Close()
}

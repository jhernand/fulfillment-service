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
	"errors"
	"fmt"
	"log/slog"
	"time"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// NotifierBuilder contains the data and logic needed to build a Kafka notifier.
type NotifierBuilder struct {
	logger   *slog.Logger
	topic    string
	producer *Producer
}

// Notifier knows how to send notifications to a Kafka topic using protocol buffers messages as payload.
type Notifier struct {
	logger   *slog.Logger
	topic    string
	producer *Producer
}

// NewNotifier creates a new Kafka notifier builder.
func NewNotifier() *NotifierBuilder {
	return &NotifierBuilder{}
}

// SetLogger sets the logger for the notifier. This is mandatory.
func (b *NotifierBuilder) SetLogger(value *slog.Logger) *NotifierBuilder {
	b.logger = value
	return b
}

// SetTopic sets the Kafka topic where notifications will be published. This is mandatory.
func (b *NotifierBuilder) SetTopic(value string) *NotifierBuilder {
	b.topic = value
	return b
}

// SetProducer sets the Kafka producer that will be used to publish notifications. This is mandatory.
func (b *NotifierBuilder) SetProducer(value *Producer) *NotifierBuilder {
	b.producer = value
	return b
}

// Build constructs a notifier instance using the configured parameters.
func (b *NotifierBuilder) Build() (result *Notifier, err error) {
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.topic == "" {
		err = errors.New("topic is mandatory")
		return
	}
	if b.producer == nil {
		err = errors.New("producer is mandatory")
		return
	}

	logger := b.logger.With(slog.String("topic", b.topic))
	result = &Notifier{
		logger:   logger,
		topic:    b.topic,
		producer: b.producer,
	}
	return
}

// Notify sends a notification with the given payload to the configured Kafka topic. The payload is wrapped into an
// Any message and serialized using protocol buffers before being published.
func (n *Notifier) Notify(ctx context.Context, payload proto.Message) error {
	// Encode the payload:
	wrapper, err := anypb.New(payload)
	if err != nil {
		return fmt.Errorf("failed to wrap payload in Any: %w", err)
	}
	data, err := proto.Marshal(wrapper)
	if err != nil {
		n.logger.ErrorContext(ctx, "Failed to serialize payload",
			slog.Any("payload", payload),
			slog.Any("error", err),
		)
		return fmt.Errorf("failed to serialize payload: %w", err)
	}

	// Produce the message:
	message := &confluentkafka.Message{
		TopicPartition: confluentkafka.TopicPartition{Topic: &n.topic, Partition: confluentkafka.PartitionAny},
		Value:          data,
		Timestamp:      time.Now(),
	}
	err = n.producer.Produce(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to produce notification: %w", err)
	}

	if n.logger.Enabled(ctx, slog.LevelDebug) {
		n.logger.DebugContext(ctx, "Sent notification",
			slog.Any("payload", payload),
		)
	}

	return nil
}

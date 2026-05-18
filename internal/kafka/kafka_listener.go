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

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ListenerBuilder contains the data and logic needed to build a Kafka listener.
type ListenerBuilder struct {
	logger   *slog.Logger
	topic    string
	consumer *Consumer
}

// Listener consumes messages from a Kafka topic, deserializes the protobuf payload, and passes it to a callback.
type Listener struct {
	logger   *slog.Logger
	topic    string
	consumer *Consumer
}

// NewListener creates a new Kafka listener builder.
func NewListener() *ListenerBuilder {
	return &ListenerBuilder{}
}

// SetLogger sets the logger for the listener. This is mandatory.
func (b *ListenerBuilder) SetLogger(value *slog.Logger) *ListenerBuilder {
	b.logger = value
	return b
}

// SetTopic sets the Kafka topic to consume from. This is mandatory.
func (b *ListenerBuilder) SetTopic(value string) *ListenerBuilder {
	b.topic = value
	return b
}

// SetConsumer sets the Kafka consumer that will be used to receive messages. This is mandatory.
func (b *ListenerBuilder) SetConsumer(value *Consumer) *ListenerBuilder {
	b.consumer = value
	return b
}

// Build constructs a listener instance using the configured parameters.
func (b *ListenerBuilder) Build() (result *Listener, err error) {
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.topic == "" {
		err = errors.New("topic is mandatory")
		return
	}
	if b.consumer == nil {
		err = errors.New("consumer is mandatory")
		return
	}

	logger := b.logger.With(slog.String("topic", b.topic))
	result = &Listener{
		logger:   logger,
		topic:    b.topic,
		consumer: b.consumer,
	}
	return
}

// Listen subscribes to the configured topic and calls the given function for each message received. It blocks until
// the context is cancelled or the underlying transport fails.
func (l *Listener) Listen(ctx context.Context, callback func(ctx context.Context, payload proto.Message) error) error {
	err := l.consumer.Subscribe(l.topic)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic '%s': %w", l.topic, err)
	}

	l.logger.InfoContext(ctx, "Listening for messages")

	for {
		select {
		case <-ctx.Done():
			l.logger.DebugContext(ctx, "Listener context cancelled")
			return ctx.Err()
		default:
		}

		event := l.consumer.Poll(1000)
		if event == nil {
			continue
		}

		switch e := event.(type) {
		case *confluentkafka.Message:
			l.processMessage(ctx, e, callback)
		case confluentkafka.Error:
			l.logger.ErrorContext(ctx, "Consumer error",
				slog.String("code", e.Code().String()),
				slog.String("error", e.Error()),
			)
		}
	}
}

// processMessage deserializes the protobuf payload from a Kafka message and invokes the callback.
func (l *Listener) processMessage(ctx context.Context, message *confluentkafka.Message,
	callback func(ctx context.Context, payload proto.Message) error) {
	l.logger.DebugContext(ctx, "Received message",
		slog.Int("partition", int(message.TopicPartition.Partition)),
		slog.Int64("offset", int64(message.TopicPartition.Offset)),
	)

	// Unwrap the payload:
	wrapper := &anypb.Any{}
	err := proto.Unmarshal(message.Value, wrapper)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to unmarshal message",
			slog.Any("error", err),
		)
		return
	}
	payload, err := wrapper.UnmarshalNew()
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to unwrap payload",
			slog.Any("error", err),
		)
		return
	}

	if l.logger.Enabled(ctx, slog.LevelDebug) {
		l.logger.DebugContext(ctx, "Decoded payload",
			slog.Any("payload", payload),
		)
	}

	err = callback(ctx, payload)
	if err != nil {
		l.logger.ErrorContext(ctx, "Callback failed",
			slog.Any("error", err),
		)
	}
}

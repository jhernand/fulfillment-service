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
	"time"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var _ = Describe("Listener", func() {
	const topic = "test-topic"

	Describe("Creation", func() {
		It("Can be created when all the required parameters are set", func() {
			consumer := &Consumer{logger: logger}
			listener, err := NewListener().
				SetLogger(logger).
				SetTopic(topic).
				SetConsumer(consumer).
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(listener).ToNot(BeNil())
		})

		It("Can't be created without a logger", func() {
			consumer := &Consumer{logger: logger}
			listener, err := NewListener().
				SetTopic(topic).
				SetConsumer(consumer).
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
			Expect(listener).To(BeNil())
		})

		It("Can't be created without a topic", func() {
			consumer := &Consumer{logger: logger}
			listener, err := NewListener().
				SetLogger(logger).
				SetConsumer(consumer).
				Build()
			Expect(err).To(MatchError("topic is mandatory"))
			Expect(listener).To(BeNil())
		})

		It("Can't be created without a consumer", func() {
			listener, err := NewListener().
				SetLogger(logger).
				SetTopic(topic).
				Build()
			Expect(err).To(MatchError("consumer is mandatory"))
			Expect(listener).To(BeNil())
		})
	})

	Describe("Behaviour", func() {
		var (
			mockCluster *confluentkafka.MockCluster
		)

		BeforeEach(func() {
			var err error
			mockCluster, err = confluentkafka.NewMockCluster(1)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(mockCluster.Close)

			err = mockCluster.CreateTopic(topic, 1, 1)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Receives one notification", func() {
			// Produce a message first (before consumer subscribes) so it's available from offset 0:
			producer, err := confluentkafka.NewProducer(&confluentkafka.ConfigMap{
				"bootstrap.servers": mockCluster.BootstrapServers(),
			})
			Expect(err).ToNot(HaveOccurred())
			defer producer.Close()

			sent := wrapperspb.String("hello kafka")
			wrapper, err := anypb.New(sent)
			Expect(err).ToNot(HaveOccurred())
			data, err := proto.Marshal(wrapper)
			Expect(err).ToNot(HaveOccurred())

			deliveryChan := make(chan confluentkafka.Event)
			topicName := topic
			err = producer.Produce(&confluentkafka.Message{
				TopicPartition: confluentkafka.TopicPartition{Topic: &topicName, Partition: confluentkafka.PartitionAny},
				Value:          data,
			}, deliveryChan)
			Expect(err).ToNot(HaveOccurred())
			e := <-deliveryChan
			m := e.(*confluentkafka.Message)
			Expect(m.TopicPartition.Error).ToNot(HaveOccurred())

			// Create a raw consumer connected to the mock cluster:
			rawConsumer, err := confluentkafka.NewConsumer(&confluentkafka.ConfigMap{
				"bootstrap.servers": mockCluster.BootstrapServers(),
				"group.id":          "test-group",
				"auto.offset.reset": "earliest",
			})
			Expect(err).ToNot(HaveOccurred())
			consumer := NewConsumerFromRaw(logger, rawConsumer)
			DeferCleanup(consumer.Close)

			// Create the listener:
			payloads := make(chan proto.Message, 1)
			listener, err := NewListener().
				SetLogger(logger).
				SetTopic(topic).
				SetConsumer(consumer).
				Build()
			Expect(err).ToNot(HaveOccurred())

			// Start the listener:
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			go func() {
				defer GinkgoRecover()
				_ = listener.Listen(ctx, func(ctx context.Context, payload proto.Message) error {
					payloads <- payload
					return nil
				})
			}()

			// Verify the listener received the message:
			var received *wrapperspb.StringValue
			Eventually(payloads, 5*time.Second).Should(Receive(&received))
			Expect(proto.Equal(received, sent)).To(BeTrue())
		})

		It("Receives multiple notifications", func() {
			// Produce multiple messages first:
			producer, err := confluentkafka.NewProducer(&confluentkafka.ConfigMap{
				"bootstrap.servers": mockCluster.BootstrapServers(),
			})
			Expect(err).ToNot(HaveOccurred())
			defer producer.Close()

			sent := []string{
				"cero",
				"uno",
				"dos",
				"tres",
				"cuatro",
			}
			for _, value := range sent {
				payload := wrapperspb.String(value)
				wrapper, err := anypb.New(payload)
				Expect(err).ToNot(HaveOccurred())
				data, err := proto.Marshal(wrapper)
				Expect(err).ToNot(HaveOccurred())

				deliveryChan := make(chan confluentkafka.Event)
				topicName := topic
				err = producer.Produce(&confluentkafka.Message{
					TopicPartition: confluentkafka.TopicPartition{
						Topic:     &topicName,
						Partition: confluentkafka.PartitionAny,
					},
					Value: data,
				}, deliveryChan)
				Expect(err).ToNot(HaveOccurred())
				e := <-deliveryChan
				m := e.(*confluentkafka.Message)
				Expect(m.TopicPartition.Error).ToNot(HaveOccurred())
			}

			// Create a raw consumer connected to the mock cluster:
			rawConsumer, err := confluentkafka.NewConsumer(&confluentkafka.ConfigMap{
				"bootstrap.servers": mockCluster.BootstrapServers(),
				"group.id":          "test-group-multi",
				"auto.offset.reset": "earliest",
			})
			Expect(err).ToNot(HaveOccurred())
			consumer := NewConsumerFromRaw(logger, rawConsumer)
			DeferCleanup(consumer.Close)

			// Create the listener:
			payloads := make(chan proto.Message, 10)
			listener, err := NewListener().
				SetLogger(logger).
				SetTopic(topic).
				SetConsumer(consumer).
				Build()
			Expect(err).ToNot(HaveOccurred())

			// Start the listener:
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			go func() {
				defer GinkgoRecover()
				_ = listener.Listen(ctx, func(ctx context.Context, payload proto.Message) error {
					payloads <- payload
					return nil
				})
			}()

			// Verify all messages are received:
			var received []string
			for range sent {
				var msg *wrapperspb.StringValue
				Eventually(payloads, 5*time.Second).Should(Receive(&msg))
				received = append(received, msg.Value)
			}
			Expect(received).To(Equal(sent))
		})
	})
})

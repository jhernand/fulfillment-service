/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package listen

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/proto"

	"github.com/osac-project/fulfillment-service/internal/kafka"
	"github.com/osac-project/fulfillment-service/internal/logging"
)

func Cmd() *cobra.Command {
	runner := &runnerContext{}
	command := &cobra.Command{
		Use:   "listen",
		Short: "Listens for notifications",
		Args:  cobra.NoArgs,
		RunE:  runner.run,
	}
	flags := command.Flags()
	kafka.AddFlags(flags)
	flags.StringVar(
		&runner.args.topic,
		"topic",
		"events",
		"Name of the Kafka topic to consume from",
	)
	flags.StringVar(
		&runner.args.groupId,
		"group-id",
		"dev-listen",
		"Consumer group identifier",
	)
	return command
}

type runnerContext struct {
	logger *slog.Logger
	flags  *pflag.FlagSet
	args   struct {
		topic   string
		groupId string
	}
}

func (c *runnerContext) run(cmd *cobra.Command, argv []string) error {
	// Get the context:
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Get the dependencies from the context:
	c.logger = logging.LoggerFromContext(ctx)

	// Save the flags:
	c.flags = cmd.Flags()

	// Check flags:
	if c.args.topic == "" {
		return errors.New("topic is mandatory")
	}
	if c.args.groupId == "" {
		return errors.New("group identifier is mandatory")
	}

	// Create the Kafka tool:
	c.logger.InfoContext(ctx, "Creating Kafka tool")
	kafkaTool, err := kafka.NewTool().
		SetLogger(c.logger).
		SetFlags(c.flags).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create Kafka tool: %w", err)
	}

	// Create the consumer:
	c.logger.InfoContext(ctx, "Creating Kafka consumer")
	consumer, err := kafkaTool.Consumer(ctx, c.args.groupId)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}
	defer consumer.Close()

	// Create the listener:
	c.logger.InfoContext(ctx, "Creating listener")
	listener, err := kafka.NewListener().
		SetLogger(c.logger).
		SetTopic(c.args.topic).
		SetConsumer(consumer).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	// Listen in a goroutine so we can handle signals:
	go func() {
		err := listener.Listen(ctx, c.processPayload)
		if err == nil || errors.Is(err, context.Canceled) {
			c.logger.InfoContext(ctx, "Listener finished")
		} else {
			c.logger.ErrorContext(ctx, "Listener failed",
				slog.Any("error", err),
			)
		}
	}()

	// Wait for a signal:
	c.logger.InfoContext(ctx, "Waiting for signal")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	c.logger.InfoContext(ctx, "Signal received, shutting down")
	cancel()
	return nil
}

func (c *runnerContext) processPayload(ctx context.Context, payload proto.Message) error {
	c.logger.InfoContext(ctx, "Received payload",
		slog.String("topic", c.args.topic),
		slog.Any("payload", payload),
	)
	return nil
}

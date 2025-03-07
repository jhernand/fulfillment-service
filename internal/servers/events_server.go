/*
Copyright 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package servers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	api "github.com/innabox/fulfillment-service/internal/api/fulfillment/v1"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
)

type EventsServerBuilder struct {
	logger *slog.Logger
	flags  *pflag.FlagSet
}

var _ api.EventsServer = (*EventsServer)(nil)

type EventsServer struct {
	api.UnimplementedEventsServer

	logger *slog.Logger
}

func NewEventsServer() *EventsServerBuilder {
	return &EventsServerBuilder{}
}

func (b *EventsServerBuilder) SetLogger(value *slog.Logger) *EventsServerBuilder {
	b.logger = value
	return b
}

func (b *EventsServerBuilder) SetFlags(value *pflag.FlagSet) *EventsServerBuilder {
	b.flags = value
	return b
}

func (b *EventsServerBuilder) Build() (result *EventsServer, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}

	// Create and populate the object:
	result = &EventsServer{
		logger: b.logger,
	}
	return
}

func (s *EventsServer) Watch(request *api.EventsWatchRequest,
	stream grpc.ServerStreamingServer[api.EventsWatchResponse]) error {

	// TODO: Fake implementation, just for testing it.
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		err := stream.Send(&api.EventsWatchResponse{
			Event: &api.Event{
				Id:   uuid.NewString(),
				Type: api.Event_CREATE,
				Old:  nil,
				New:  nil,
			},
		})
		if err != nil {
			s.logger.ErrorContext(
				ctx,
				"Failed to send event",
				slog.String("error", err.Error()),
			)
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

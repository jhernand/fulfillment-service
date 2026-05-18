/*
Copyright (c) 2026 Red Hat Inc.

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

	"google.golang.org/protobuf/proto"
)

// Listener is the interface for components that can receive change notifications.
type Listener interface {
	// Listen blocks and consumes messages, calling the given function for each message received. It returns when
	// the context is cancelled or the underlying transport fails.
	Listen(ctx context.Context, callback func(ctx context.Context, payload proto.Message) error) error
}

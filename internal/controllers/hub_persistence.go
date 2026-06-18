/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// statusWithHub is satisfied by any protobuf status type that carries a hub field.
// All generated *Status types (ClusterStatus, SubnetStatus, etc.) implicitly satisfy
// this via Go's structural typing.
type statusWithHub interface {
	GetHub() string
	SetHub(string)
}

// HubPersistenceHelper persists hub selection to the database before creating
// Kubernetes objects, preventing orphaned objects on crash.
type HubPersistenceHelper struct {
	logger     *slog.Logger
	objectId   string
	status     statusWithHub
	selectHub  func(context.Context) (string, error)
	persistHub func(context.Context) error
}

// HubPersistenceHelperBuilder builds HubPersistenceHelper instances.
type HubPersistenceHelperBuilder struct {
	logger     *slog.Logger
	objectId   string
	status     statusWithHub
	selectHub  func(context.Context) (string, error)
	persistHub func(context.Context) error
}

// NewHubPersistenceHelper creates a new builder for constructing a HubPersistenceHelper.
func NewHubPersistenceHelper() *HubPersistenceHelperBuilder {
	return &HubPersistenceHelperBuilder{}
}

// SetLogger sets the logger. This is mandatory.
func (b *HubPersistenceHelperBuilder) SetLogger(value *slog.Logger) *HubPersistenceHelperBuilder {
	b.logger = value
	return b
}

// SetObjectId sets the identifier of the object being reconciled (for logging).
func (b *HubPersistenceHelperBuilder) SetObjectId(value string) *HubPersistenceHelperBuilder {
	b.objectId = value
	return b
}

// SetStatus sets the status object that carries the hub field. This is mandatory.
func (b *HubPersistenceHelperBuilder) SetStatus(value statusWithHub) *HubPersistenceHelperBuilder {
	b.status = value
	return b
}

// SetSelectHub sets the function that selects a hub. This is mandatory.
func (b *HubPersistenceHelperBuilder) SetSelectHub(value func(context.Context) (string, error)) *HubPersistenceHelperBuilder {
	b.selectHub = value
	return b
}

// SetPersistHub sets the function that persists the hub to the database. This is mandatory.
func (b *HubPersistenceHelperBuilder) SetPersistHub(value func(context.Context) error) *HubPersistenceHelperBuilder {
	b.persistHub = value
	return b
}

// Build validates configuration and creates a HubPersistenceHelper.
func (b *HubPersistenceHelperBuilder) Build() (*HubPersistenceHelper, error) {
	if b.logger == nil {
		return nil, errors.New("logger is mandatory")
	}
	if b.status == nil {
		return nil, errors.New("status is mandatory")
	}
	if b.selectHub == nil {
		return nil, errors.New("selectHub function is mandatory")
	}
	if b.persistHub == nil {
		return nil, errors.New("persistHub function is mandatory")
	}
	return &HubPersistenceHelper{
		logger:     b.logger,
		objectId:   b.objectId,
		status:     b.status,
		selectHub:  b.selectHub,
		persistHub: b.persistHub,
	}, nil
}

// Run selects a hub and immediately persists it to the database.
// If the hub is already set, it returns immediately.
func (h *HubPersistenceHelper) Run(ctx context.Context) error {
	if h.status.GetHub() != "" {
		h.logger.DebugContext(ctx, "Hub already set, skipping persistence",
			slog.String("object_id", h.objectId),
			slog.String("hub_id", h.status.GetHub()),
		)
		return nil
	}

	hubID, err := h.selectHub(ctx)
	if err != nil {
		return fmt.Errorf("failed to select hub for %s: %w", h.objectId, err)
	}
	if hubID == "" {
		return fmt.Errorf("selectHub returned empty hub ID for %s", h.objectId)
	}

	h.status.SetHub(hubID)

	if err := h.persistHub(ctx); err != nil {
		return fmt.Errorf("failed to persist hub selection for %s: %w", h.objectId, err)
	}

	h.logger.DebugContext(ctx, "Persisted hub selection",
		slog.String("object_id", h.objectId),
		slog.String("hub_id", hubID),
	)

	return nil
}

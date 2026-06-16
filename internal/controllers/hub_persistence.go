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
	"fmt"
	"log/slog"
)

// HubPersistenceHelper persists hub selection to the database BEFORE creating
// Kubernetes CRs, preventing orphaned CRs on crash (OSAC-455).
type HubPersistenceHelper struct {
	logger *slog.Logger
}

// NewHubPersistenceHelper creates a new helper for persisting hub selections.
func NewHubPersistenceHelper(logger *slog.Logger) *HubPersistenceHelper {
	return &HubPersistenceHelper{
		logger: logger,
	}
}

// SelectAndPersistHub selects a hub and immediately persists it to the database.
// Must be called BEFORE creating the Kubernetes CR.
func (h *HubPersistenceHelper) SelectAndPersistHub(
	ctx context.Context,
	resourceID string,
	resourceType string,
	getCurrentHub func() string,
	selectHub func(context.Context) (string, error),
	setHub func(string),
	persistHub func(context.Context) error,
) error {
	if getCurrentHub == nil || selectHub == nil || setHub == nil || persistHub == nil {
		return fmt.Errorf("all function parameters must be non-nil")
	}

	currentHub := getCurrentHub()
	if currentHub != "" {
		h.logger.DebugContext(
			ctx,
			"Hub already set, skipping persistence",
			slog.String("resource_type", resourceType),
			slog.String("resource_id", resourceID),
			slog.String("hub_id", currentHub),
		)
		return nil
	}

	hubID, err := selectHub(ctx)
	if err != nil {
		return fmt.Errorf("failed to select hub for %s %s: %w", resourceType, resourceID, err)
	}

	if hubID == "" {
		return fmt.Errorf("selectHub returned empty hub ID for %s %s", resourceType, resourceID)
	}

	setHub(hubID)

	err = persistHub(ctx)
	if err != nil {
		return fmt.Errorf("failed to persist hub selection for %s %s: %w", resourceType, resourceID, err)
	}

	h.logger.InfoContext(
		ctx,
		"Persisted hub selection before CR creation",
		slog.String("resource_type", resourceType),
		slog.String("resource_id", resourceID),
		slog.String("hub_id", hubID),
	)

	return nil
}

// ShouldPersistHub returns true if the resource has no hub and is in a state
// where hub selection should happen.
func (h *HubPersistenceHelper) ShouldPersistHub(
	getCurrentHub func() string,
	isProgressing func() bool,
) bool {
	return getCurrentHub() == "" && isProgressing()
}

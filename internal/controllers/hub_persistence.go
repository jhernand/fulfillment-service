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

// HubPersistenceHelper encapsulates the logic for persisting hub selection to the database
// BEFORE creating Kubernetes CRs. This prevents orphaning CRs when the reconciler crashes
// between CR creation and database persistence (OSAC-455).
//
// All reconcilers that manage hub-placed resources should use this helper to ensure
// hub selection is persisted atomically before creating side effects.
type HubPersistenceHelper struct {
	logger *slog.Logger
}

// NewHubPersistenceHelper creates a new helper for persisting hub selections.
func NewHubPersistenceHelper(logger *slog.Logger) *HubPersistenceHelper {
	return &HubPersistenceHelper{
		logger: logger,
	}
}

// SelectAndPersistHub selects a hub for a resource and immediately persists the selection
// to the database. This must be called BEFORE creating the Kubernetes CR.
//
// Parameters:
//   - ctx: Context for the operation
//   - resourceID: ID of the resource being reconciled (for logging)
//   - resourceType: Type of resource (e.g., "cluster", "computeinstance") for logging
//   - getCurrentHub: Function that returns the current hub from the resource status
//   - selectHub: Function that selects a hub (typically calls selectHub() on the task)
//   - setHub: Function that sets the hub in the resource status (in-memory)
//   - persistHub: Function that persists the hub to the database with field mask ["status.hub"]
//
// Returns:
//   - error if hub selection or persistence fails
//
// Example usage in a reconciler's run() function:
//
//	helper := controllers.NewHubPersistenceHelper(r.logger)
//	err := helper.SelectAndPersistHub(
//	    ctx,
//	    cluster.GetId(),
//	    "cluster",
//	    func() string { return cluster.GetStatus().GetHub() },
//	    func(ctx context.Context) (string, error) {
//	        return t.selectHubInternal(ctx)
//	    },
//	    func(hubID string) { cluster.GetStatus().SetHub(hubID) },
//	    func(ctx context.Context) error {
//	        _, err := r.clustersClient.Update(ctx, privatev1.ClustersUpdateRequest_builder{
//	            Object: cluster,
//	            UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status.hub"}},
//	        }.Build())
//	        return err
//	    },
//	)
func (h *HubPersistenceHelper) SelectAndPersistHub(
	ctx context.Context,
	resourceID string,
	resourceType string,
	getCurrentHub func() string,
	selectHub func(context.Context) (string, error),
	setHub func(string),
	persistHub func(context.Context) error,
) error {
	// Validate required parameters
	if getCurrentHub == nil || selectHub == nil || setHub == nil || persistHub == nil {
		return fmt.Errorf("all function parameters must be non-nil")
	}

	// Check if hub is already selected
	currentHub := getCurrentHub()
	if currentHub != "" {
		// Hub already set, nothing to persist
		h.logger.DebugContext(
			ctx,
			"Hub already set, skipping persistence",
			slog.String("resource_type", resourceType),
			slog.String("resource_id", resourceID),
			slog.String("hub_id", currentHub),
		)
		return nil
	}

	// Select hub
	hubID, err := selectHub(ctx)
	if err != nil {
		return fmt.Errorf("failed to select hub for %s %s: %w", resourceType, resourceID, err)
	}

	// Validate hub selection returned a non-empty ID
	if hubID == "" {
		return fmt.Errorf("selectHub returned empty hub ID for %s %s", resourceType, resourceID)
	}

	// Save hub to status (in-memory)
	setHub(hubID)

	// Persist to database immediately (only status.hub field)
	err = persistHub(ctx)
	if err != nil {
		return fmt.Errorf("failed to persist hub selection for %s %s: %w", resourceType, resourceID, err)
	}

	h.logger.InfoContext(
		ctx,
		"Persisted hub selection to database before creating CR (OSAC-455 fix)",
		slog.String("resource_type", resourceType),
		slog.String("resource_id", resourceID),
		slog.String("hub_id", hubID),
	)

	return nil
}

// ShouldPersistHub determines whether hub persistence is needed for a resource.
// Returns true if the resource needs hub selection and persistence.
//
// Parameters:
//   - getCurrentHub: Function that returns the current hub from the resource status
//   - isProgressing: Function that returns true if the resource is in a progressing/pending state
//
// This helper method can be used in the run() function to decide whether to call
// SelectAndPersistHub.
func (h *HubPersistenceHelper) ShouldPersistHub(
	getCurrentHub func() string,
	isProgressing func() bool,
) bool {
	return getCurrentHub() == "" && isProgressing()
}

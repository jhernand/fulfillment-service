/*
Copyright 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in
compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is
distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing permissions and limitations under the
License.
*/

package dao

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/innabox/fulfillment-service/internal/database/models"
)

type ClusterTemplate interface {
	List(ctx context.Context) (items []*models.ClusterTemplate, err error)
	Get(ctx context.Context, id string) (item *models.ClusterTemplate, err error)
}

type ClusterTemplateBuilder struct {
	logger *slog.Logger
	pool   *pgxpool.Pool
}

type clusterTemplate struct {
	base
}

func NewClusterTemplate() *ClusterTemplateBuilder {
	return &ClusterTemplateBuilder{}
}

func (b *ClusterTemplateBuilder) SetLogger(value *slog.Logger) *ClusterTemplateBuilder {
	b.logger = value
	return b
}

func (b *ClusterTemplateBuilder) SetPool(value *pgxpool.Pool) *ClusterTemplateBuilder {
	b.pool = value
	return b
}

func (b *ClusterTemplateBuilder) Build() (result ClusterTemplate, err error) {
	// Check parameters:
	if b.logger == nil {
		err = fmt.Errorf("logger is mandatory")
		return
	}
	if b.pool == nil {
		err = fmt.Errorf("database connection pool is mandatory")
		return
	}

	// Create and populate the object:
	result = &clusterTemplate{
		base: base{
			logger: b.logger,
			pool:   b.pool,
		},
	}
	return
}

func (d *clusterTemplate) List(ctx context.Context) (items []*models.ClusterTemplate, err error) {
	// Start a transaction:
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return
	}
	defer func() {
		err := tx.Rollback(ctx)
		if err != nil {
			d.logger.ErrorContext(
				ctx,
				"Failed to rollback transaction",
				slog.String("error", err.Error()),
			)
		}
	}()

	// Fetch the results:
	rows, err := tx.Query(
		ctx,
		"select id, title, description from cluster_templates",
	)
	if err != nil {
		return
	}
	var tmp []*models.ClusterTemplate
	for rows.Next() {
		var (
			id          string
			title       string
			description string
		)
		err = rows.Scan(&id, &title, &description)
		if err != nil {
			return
		}
		tmp = append(tmp, &models.ClusterTemplate{
			ID:          id,
			Title:       title,
			Description: description,
		})
	}

	// Return the result:
	items = tmp
	return
}

func (d *clusterTemplate) Get(ctx context.Context, id string) (item *models.ClusterTemplate, err error) {
	// Start a transaction:
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return
	}
	defer func() {
		err := tx.Rollback(ctx)
		if err != nil {
			d.logger.ErrorContext(
				ctx,
				"Failed to rollback transaction",
				slog.String("error", err.Error()),
			)
		}
	}()

	// Fetch the results:
	row := tx.QueryRow(
		ctx,
		"select title, description from cluster_templates where id = $1",
	)
	tmp := &models.ClusterTemplate{
		ID: id,
	}
	err = row.Scan(&tmp.Title, &tmp.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
		return
	}
	item = tmp
	return
}

/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/osac-project/fulfillment-service/internal/database"
)

// request is a common base for all DAO request types, containing shared fields.
type request[O Object] struct {
	dao *GenericDAO[O]
	tx  database.Tx
	sql struct {
		filter strings.Builder
		params []any
	}
}

type archiveArgs struct {
	id              string
	creationTs      time.Time
	deletionTs      time.Time
	creator         string
	tenant          string
	name            string
	labelsData      []byte
	annotationsData []byte
	version         int32
	data            []byte
}

// archive moves a deleted object to the archived table and removes it from the main table.
func (r *request[O]) archive(ctx context.Context, args archiveArgs) error {
	sql := fmt.Sprintf(
		`
		insert into archived_%s (
			id,
			name,
			creation_timestamp,
			deletion_timestamp,
			creator,
			tenant,
			labels,
			annotations,
			version,
			data
		) values (
		 	$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10
		)
		`,
		r.dao.table,
	)
	_, err := r.exec(
		ctx,
		archiveOpType,
		sql,
		args.id,
		args.name,
		args.creationTs,
		args.deletionTs,
		args.creator,
		args.tenant,
		args.labelsData,
		args.annotationsData,
		args.version,
		args.data,
	)
	if err != nil {
		return err
	}
	sql = fmt.Sprintf(`delete from %s where id = $1`, r.dao.table)
	_, err = r.exec(ctx, deleteOpType, sql, args.id)
	return err
}

type makeMetadataArgs struct {
	creationTs  time.Time
	deletionTs  time.Time
	finalizers  []string
	creator     string
	tenant      string
	name        string
	labels      map[string]string
	annotations map[string]string
	version     int32
}

func (r *request[O]) makeMetadata(args makeMetadataArgs) metadataIface {
	result := r.dao.metadataTemplate.New().Interface().(metadataIface)
	result.SetName(args.name)
	if args.creationTs.Unix() != 0 {
		result.SetCreationTimestamp(timestamppb.New(args.creationTs))
	}
	if args.deletionTs.Unix() != 0 {
		result.SetDeletionTimestamp(timestamppb.New(args.deletionTs))
	}
	result.SetFinalizers(args.finalizers)
	result.SetCreator(args.creator)
	result.SetTenant(args.tenant)
	result.SetLabels(args.labels)
	result.SetAnnotations(args.annotations)
	result.SetVersion(args.version)
	return result
}

func (r *request[O]) getMetadata(object O) metadataIface {
	objectReflect := object.ProtoReflect()
	if !objectReflect.Has(r.dao.metadataField) {
		return nil
	}
	return objectReflect.Get(r.dao.metadataField).Message().Interface().(metadataIface)
}

func (r *request[O]) setMetadata(object O, metadata metadataIface) {
	objectReflect := object.ProtoReflect()
	if metadata != nil {
		metadataReflect := metadata.ProtoReflect()
		objectReflect.Set(r.dao.metadataField, protoreflect.ValueOfMessage(metadataReflect))
	} else {
		objectReflect.Clear(r.dao.metadataField)
	}
}

func (r *request[O]) newObject() O {
	return r.dao.objectTemplate.New().Interface().(O)
}

func (r *request[O]) cloneObject(object O) O {
	return proto.Clone(object).(O)
}

func (r *request[O]) marshalData(object O) (result []byte, err error) {
	result, err = r.dao.jsonEncoder.Marshal(object)
	return
}

func (r *request[O]) unmarshalData(data []byte, object O) error {
	return r.dao.unmarshalOptions.Unmarshal(data, object)
}

func (r *request[O]) fireEvent(ctx context.Context, event Event) error {
	event.Table = r.dao.table
	for _, eventCallback := range r.dao.eventCallbacks {
		err := eventCallback(ctx, event)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *request[O]) getFinalizers(metadata metadataIface) []string {
	if metadata == nil {
		return []string{}
	}
	list := metadata.GetFinalizers()
	set := make(map[string]struct{}, len(list))
	for _, item := range list {
		set[item] = struct{}{}
	}
	list = make([]string, len(set))
	i := 0
	for item := range set {
		list[i] = item
		i++
	}
	sort.Strings(list)
	return list
}

func (r *request[O]) marshalMap(value map[string]string) (result []byte, err error) {
	if value == nil {
		result = []byte("{}")
		return
	}
	result, err = json.Marshal(value)
	return
}

func (r *request[O]) unmarshalMap(data []byte) (result map[string]string, err error) {
	if len(data) == 0 {
		return
	}
	var value map[string]string
	err = json.Unmarshal(data, &value)
	if err != nil {
		return
	}
	result = value
	return
}

// queryRow executes a SQL query expected to return a single row. It logs the SQL statement before delegating to the
// underlying transaction.
func (r *request[O]) queryRow(ctx context.Context, op opType, sql string, args ...any) pgx.Row {
	if r.dao.logger.Enabled(ctx, slog.LevelDebug) {
		r.dao.logger.DebugContext(
			ctx,
			"Running SQL operation",
			slog.String("type", string(op)),
			slog.String("sql", r.cleanSQL(sql)),
			slog.Any("parameters", args),
		)
	}
	return r.tx.QueryRow(ctx, sql, args...)
}

// query executes a SQL query expected to return multiple rows. It logs the SQL statement before delegating to the
// underlying transaction.
func (r *request[O]) query(ctx context.Context, op opType, sql string, args ...any) (rows pgx.Rows, err error) {
	if r.dao.logger.Enabled(ctx, slog.LevelDebug) {
		r.dao.logger.DebugContext(
			ctx,
			"Running SQL operation",
			slog.String("type", string(op)),
			slog.String("sql", r.cleanSQL(sql)),
			slog.Any("parameters", args),
		)
	}
	rows, err = r.tx.Query(ctx, sql, args...)
	return
}

// exec executes a SQL statement that doesn't return rows. It logs the SQL statement before delegating to the
// underlying transaction.
func (r *request[O]) exec(ctx context.Context, op opType, sql string, args ...any) (pgconn.CommandTag, error) {
	if r.dao.logger.Enabled(ctx, slog.LevelDebug) {
		r.dao.logger.DebugContext(
			ctx,
			"Running SQL operation",
			slog.String("type", string(op)),
			slog.String("sql", r.cleanSQL(sql)),
			slog.Any("parameters", args),
		)
	}
	start := time.Now()
	tag, err := r.tx.Exec(ctx, sql, args...)
	r.recordOpDuration(op, start, err)
	return tag, err
}

// recordOpDuration records the elapsed time since start as a Prometheus histogram observation, if metrics are
// configured. The err parameter is the error returned by the SQL operation. When it is nil the `error` label will be
// empty, otherwise it will contain the PostgreSQL error code.
func (r *request[O]) recordOpDuration(op opType, start time.Time, err error) {
	if r.dao.opDurationMetric != nil {
		code := ""
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				code = pgErr.Code
			}
		}
		r.dao.opDurationMetric.With(prometheus.Labels{
			errorMetricLabel: code,
			tableMetricLabel: r.dao.table,
			typeMetricLabel:  string(op),
		}).Observe(time.Since(start).Seconds())
	}
}

// cleanSQL collapses all sequences of whitespace in the given SQL string into a single space, producing a
// compact single-line representation suitable for logging.
func (r *request[O]) cleanSQL(sql string) string {
	var buf strings.Builder
	buf.Grow(len(sql))
	space := true
	for _, c := range sql {
		if unicode.IsSpace(c) {
			if !space {
				buf.WriteRune(' ')
				space = true
			}
		} else {
			buf.WriteRune(c)
			space = false
		}
	}
	result := buf.String()
	if space && len(result) > 0 {
		result = result[:len(result)-1]
	}
	return result
}

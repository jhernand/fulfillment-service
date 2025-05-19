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
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	sharedv1 "github.com/innabox/fulfillment-service/internal/api/shared/v1"
	"github.com/innabox/fulfillment-service/internal/database"
)

// Object is the interface that should be satisfied by objects to be managed by the generic DAO.
type Object interface {
	proto.Message
	GetId() string
	SetId(string)
	GetMetadata() *sharedv1.Metadata
	SetMetadata(*sharedv1.Metadata)
}

// GenericDAOBuilder is a builder for creating generic data access objects.
type GenericDAOBuilder[O Object] struct {
	logger         *slog.Logger
	table          string
	defaultOrder   string
	defaultLimit   int
	maxLimit       int
	eventCallbacks []EventCallback
}

// GenericDAO provides generic data access operations for protocol buffers messages. It assumes that objects will be
// stored in tables with the following columns:
//
//   - `id` - The unique identifier of the object.
//   - `creation_timestamp` - The time the object was created.
//   - `deletion_timestamp` - The time the object was deleted.
//   - `data` - The serialized object, using the protocol buffers JSON serialization.
//
// Objects must have field named `id` of string type.
type GenericDAO[O Object] struct {
	logger           *slog.Logger
	table            string
	defaultOrder     string
	defaultLimit     int
	maxLimit         int
	eventCallbacks   []EventCallback
	objectTemplate   O
	marshalOptions   protojson.MarshalOptions
	filterTranslator *FilterTranslator[O]
}

// NewGenericDAO creates a builder that can then be used to configure and create a generic DAO.
func NewGenericDAO[O Object]() *GenericDAOBuilder[O] {
	return &GenericDAOBuilder[O]{
		defaultLimit: 100,
		maxLimit:     1000,
	}
}

// SetLogger sets the logger. This is mandatory.
func (b *GenericDAOBuilder[O]) SetLogger(value *slog.Logger) *GenericDAOBuilder[O] {
	b.logger = value
	return b
}

// SetTable sets the table name. This is mandatory.
func (b *GenericDAOBuilder[O]) SetTable(value string) *GenericDAOBuilder[O] {
	b.table = value
	return b
}

// SetDefaultOrder sets the default order criteria to use when nothing has been requested by the user. This is optional
// and the default is no order. This is intended only for use in unit tests, where it is convenient to have some
// predictable ordering.
func (b *GenericDAOBuilder[O]) SetDefaultOrder(value string) *GenericDAOBuilder[O] {
	b.defaultOrder = value
	return b
}

// SetDefaultLimit sets the default number of items returned. It will be used when the value of the limit parameter
// of the list request is zero. This is optional, and the default is 100.
func (b *GenericDAOBuilder[O]) SetDefaultLimit(value int) *GenericDAOBuilder[O] {
	b.defaultLimit = value
	return b
}

// SetMaxLimit sets the maximum number of items returned. This is optional and the default value is 1000.
func (b *GenericDAOBuilder[O]) SetMaxLimit(value int) *GenericDAOBuilder[O] {
	b.maxLimit = value
	return b
}

// AddEventCallback adds a function that will be called to process events when the DAO creates, updates or deletes
// an object.
//
// The functions are called synchronously, in the same order they were added, and with the same context used by the
// DAO for its operations. If any of them returns an error the transaction will be rolled back.
func (b *GenericDAOBuilder[O]) AddEventCallback(value EventCallback) *GenericDAOBuilder[O] {
	b.eventCallbacks = append(b.eventCallbacks, value)
	return b
}

// Build creates a new generic DAO using the configuration stored in the builder.
func (b *GenericDAOBuilder[O]) Build() (result *GenericDAO[O], err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.table == "" {
		err = errors.New("table is mandatory")
		return
	}
	if b.defaultLimit <= 0 {
		err = fmt.Errorf("default limit must be a possitive integer, but it is %d", b.defaultLimit)
		return
	}
	if b.maxLimit <= 0 {
		err = fmt.Errorf("max limit must be a possitive integer, but it is %d", b.maxLimit)
		return
	}
	if b.maxLimit < b.defaultLimit {
		err = fmt.Errorf(
			"max limit must be greater or equal to default limit, but max limit is %d and default limit "+
				"is %d",
			b.maxLimit, b.defaultLimit,
		)
		return
	}

	// Create the template that we will clone when we need to create a new object:
	var object O
	objectTemplate := object.ProtoReflect().New().Interface().(O)

	// Prepare the JSON marshalling options:
	marshalOptions := protojson.MarshalOptions{
		UseProtoNames: true,
	}

	// Create the filter translator:
	filterTranslator, err := NewFilterTranslator[O]().
		SetLogger(b.logger).
		Build()
	if err != nil {
		err = fmt.Errorf("failed to create filter translator: %w", err)
		return
	}

	// Create and populate the object:
	result = &GenericDAO[O]{
		logger:           b.logger,
		table:            b.table,
		defaultOrder:     b.defaultOrder,
		defaultLimit:     b.defaultLimit,
		maxLimit:         b.maxLimit,
		eventCallbacks:   slices.Clone(b.eventCallbacks),
		objectTemplate:   objectTemplate,
		marshalOptions:   marshalOptions,
		filterTranslator: filterTranslator,
	}
	return
}

// ListResponse represents the result of a paginated query.
type ListResponse[O Object] interface {
	// GetSize returns the actual number of items returned.
	GetSize() int

	// GetTotal returns the total number of items available.
	GetTotal() int

	// GetItems returns the list of items.
	GetItems() []O
}

type listResponse[O Object] struct {
	items []O
	total int
}

func (r listResponse[O]) GetSize() int {
	return len(r.items)
}

func (r listResponse[O]) GetTotal() int {
	return r.total
}

func (r listResponse[O]) GetItems() []O {
	return r.items
}

// ListRequest represents the parameters for paginated queries.
type ListRequest[O Object] interface {
	// SetOffset sets the starting point.
	SetOffset(int) ListRequest[O]

	// SetLimit sets the maximum number of items.
	SetLimit(int) ListRequest[O]

	// SetFilter sets the CEL expression that defines which objects should be returned.
	SetFilter(string) ListRequest[O]

	// Send sends the request and waits for the response.
	Send(context.Context) (ListResponse[O], error)
}

type listRequest[O Object] struct {
	d      *GenericDAO[O]
	offset int
	limit  int
	filter string
}

func (r *listRequest[O]) SetOffset(value int) ListRequest[O] {
	r.offset = value
	return r
}

func (r *listRequest[O]) SetLimit(value int) ListRequest[O] {
	r.limit = value
	return r
}

func (r *listRequest[O]) SetFilter(value string) ListRequest[O] {
	r.filter = value
	return r
}

func (r *listRequest[O]) Send(ctx context.Context) (response ListResponse[O], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.list(ctx, tx, r)
	return
}

// List runs a query.
func (d *GenericDAO[O]) List() ListRequest[O] {
	return &listRequest[O]{
		d: d,
	}
}

func (d *GenericDAO[O]) list(ctx context.Context, tx database.Tx, request *listRequest[O]) (response listResponse[O],
	err error) {
	// Calculate the filter:
	var filter string
	if request.filter != "" {
		filter, err = d.filterTranslator.Translate(ctx, request.filter)
		if err != nil {
			return
		}
	}

	// Calculate the order cluase:
	var order string
	if d.defaultOrder != "" {
		order = d.defaultOrder
	}

	// Calculate the offset:
	offset := request.offset
	if offset < 0 {
		offset = 0
	}

	// Calculate the limit:
	limit := request.limit
	if limit < 0 {
		limit = 0
	} else if limit == 0 {
		limit = d.defaultLimit
	} else if limit > d.maxLimit {
		limit = d.maxLimit
	}

	// Count the total number of results, disregarding the offset and the limit:
	totalQuery := fmt.Sprintf("select count(*) from %s", d.table)
	if filter != "" {
		totalQuery += fmt.Sprintf(" where %s", filter)
	}
	d.logger.DebugContext(
		ctx,
		"Running SQL query",
		slog.String("sql", totalQuery),
	)
	totalRow := tx.QueryRow(ctx, totalQuery)
	var total int
	err = totalRow.Scan(&total)
	if err != nil {
		return
	}

	// Fetch the results:
	itemsQuery := fmt.Sprintf(
		`
		select
			id,
			creation_timestamp,
			deletion_timestamp,
			data
		from
			%s
		`,
		d.table,
	)
	if filter != "" {
		itemsQuery += fmt.Sprintf(" where %s", filter)
	}
	if order != "" {
		itemsQuery += fmt.Sprintf(" order by %s", order)
	}
	itemsQuery += " offset $1 limit $2"
	d.logger.DebugContext(
		ctx,
		"Running SQL query",
		slog.String("sql", itemsQuery),
	)
	itemsRows, err := tx.Query(ctx, itemsQuery, offset, limit)
	if err != nil {
		return
	}
	defer itemsRows.Close()
	var items []O
	for itemsRows.Next() {
		var (
			id         string
			creationTs time.Time
			deletionTs time.Time
			data       []byte
		)
		err = itemsRows.Scan(
			&id,
			&creationTs,
			&deletionTs,
			&data,
		)
		if err != nil {
			return
		}
		item := d.newObject()
		err = d.unmarshalData(data, item)
		if err != nil {
			return
		}
		md := d.makeMetadata(creationTs, deletionTs)
		item.SetId(id)
		item.SetMetadata(md)
		items = append(items, item)
	}
	err = itemsRows.Err()
	if err != nil {
		return
	}
	response.total = total
	response.items = items
	return
}

// GetResponse contains the result of fetching a single object.
type GetResponse[O Object] interface {
	// HasObject returns true if the object exists, and false otherwise.
	HasObject() bool

	// GetObject return the data of the object, or nil if no such object exists.
	GetObject() O
}

type getResponse[O Object] struct {
	object O
}

func (r getResponse[O]) HasObject() bool {
	return !isNil(r.object)
}

func (r getResponse[O]) GetObject() O {
	return r.object
}

// GetRequest contains the parameters to fetch a single object.
type GetRequest[O Object] interface {
	// SetObject sets the object to request. This is equivaent to calling SetID with the identifier of the object.
	SetObject(O) GetRequest[O]

	// SetID sets the identifier of the object.
	SetID(string) GetRequest[O]

	// Send sends the request and waits for the response. Note that no error will be returned if the object doesn't
	// exist. Use the HasObject or GetObject methods of the response to check of the object exists.
	Send(context.Context) (GetResponse[O], error)
}

type getRequest[O Object] struct {
	d  *GenericDAO[O]
	id string
}

func (r *getRequest[O]) SetObject(value O) GetRequest[O] {
	r.id = value.GetId()
	return r
}

func (r *getRequest[O]) SetID(value string) GetRequest[O] {
	r.id = value
	return r
}

func (r *getRequest[O]) Send(ctx context.Context) (response GetResponse[O], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.get(ctx, tx, r)
	return
}

// Get fetches a single object.
func (d *GenericDAO[O]) Get() GetRequest[O] {
	return &getRequest[O]{
		d: d,
	}
}

func (d *GenericDAO[O]) get(ctx context.Context, tx database.Tx, request *getRequest[O]) (response *getResponse[O],
	err error) {
	if request.id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}
	query := fmt.Sprintf(
		`
		select
			creation_timestamp,
			deletion_timestamp,
			data
		from
			%s
		where
			id = $1
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, query, request.id)
	var (
		creationTs time.Time
		deletionTs time.Time
		data       []byte
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&data,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
		return
	}
	gotten := d.newObject()
	err = d.unmarshalData(data, gotten)
	if err != nil {
		return
	}
	md := d.makeMetadata(creationTs, deletionTs)
	gotten.SetId(request.id)
	gotten.SetMetadata(md)
	response = &getResponse[O]{
		object: gotten,
	}
	return
}

// Exists checks if a row with the given identifiers exists. Returns false and no error if there is no row with the
// given identifier.
func (d *GenericDAO[O]) Exists(ctx context.Context, id string) (ok bool, err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	ok, err = d.exists(ctx, tx, id)
	return
}

func (d *GenericDAO[O]) exists(ctx context.Context, tx database.Tx, id string) (ok bool, err error) {
	if id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}
	sql := fmt.Sprintf("select count(*) from %s where id = $1", d.table)
	row := tx.QueryRow(ctx, sql, id)
	var count int
	err = row.Scan(&count)
	if err != nil {
		return
	}
	ok = count > 0
	return
}

// CreateResponse contains the results of creating an object.
type CreateResponse[O Object] interface {
	// GetObject returns the created object.
	GetObject() O
}

type createResponse[O Object] struct {
	object O
}

func (r createResponse[O]) GetObject() O {
	return r.object
}

type createRequest[O Object] struct {
	d      *GenericDAO[O]
	object O
}

type CreateRequest[O Object] interface {
	// SetObject sets the initial data of the object. This will not be modified. Use the GetObject method of the
	// response to get the created object.
	SetObject(objet O) CreateRequest[O]

	// Send sends the request and waits for the response.
	Send(ctx context.Context) (CreateResponse[O], error)
}

func (r *createRequest[O]) SetObject(object O) CreateRequest[O] {
	r.object = object
	return r
}

func (r *createRequest[O]) Send(ctx context.Context) (response CreateResponse[O], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	if isNil(r.object) {
		r.object = r.d.newObject()
	}
	response, err = r.d.create(ctx, tx, r)
	return
}

// Create creates a new object.
func (d *GenericDAO[O]) Create() CreateRequest[O] {
	return &createRequest[O]{
		d: d,
	}
}

func (d *GenericDAO[O]) create(ctx context.Context, tx database.Tx, request *createRequest[O]) (response createResponse[O],
	err error) {
	// Generate an identifier if needed:
	id := request.object.GetId()
	if id == "" {
		id = d.newId()
	}

	// Save the object:
	data, err := d.marshalData(request.object)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		insert into %s (
			id,
			data
		) values (
		 	$1,
			$2
		)
		returning
			creation_timestamp,
			deletion_timestamp
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, id, data)
	var (
		creationTs time.Time
		deletionTs time.Time
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
	)
	if err != nil {
		return
	}
	object := d.cloneObject(request.object)
	md := d.makeMetadata(creationTs, deletionTs)
	object.SetId(id)
	object.SetMetadata(md)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeCreated,
		Object: object,
	})
	if err != nil {
		return
	}

	response.object = object
	return
}

// UpdateResponse response contains the result of updating an object.
type UpdateResponse[O Object] interface {
	// GetObject returns the data of the object after the update.
	GetObject() O
}

type updateResponse[O Object] struct {
	object O
}

func (r updateResponse[O]) GetObject() O {
	return r.object
}

// UpdateRequest contains the parameters for updating an object.
type UpdateRequest[O Object] interface {
	// SetObject sets the new data of the object. This is mandatory.
	SetObject(O) UpdateRequest[O]

	// Send sends the request and waits for the response.
	Send(context.Context) (UpdateResponse[O], error)
}

type updateRequest[O Object] struct {
	d      *GenericDAO[O]
	object O
}

func (r *updateRequest[O]) SetObject(object O) UpdateRequest[O] {
	r.object = object
	return r
}

func (r *updateRequest[O]) Send(ctx context.Context) (response UpdateResponse[O], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.update(ctx, tx, r)
	return
}

// Update updates an existing object.
func (d *GenericDAO[O]) Update() UpdateRequest[O] {
	return &updateRequest[O]{
		d: d,
	}
}

func (d *GenericDAO[O]) update(ctx context.Context, tx database.Tx, request *updateRequest[O]) (response updateResponse[O],
	err error) {
	// Get the current object:
	id := request.object.GetId()
	if id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}
	getResponse, err := d.get(ctx, tx, &getRequest[O]{
		id: id,
	})
	if err != nil {
		return
	}
	current := getResponse.object

	// Do nothing if there are no changes:
	updated := d.cloneObject(request.object)
	updated.SetMetadata(nil)
	current.SetMetadata(nil)
	if proto.Equal(updated, current) {
		return
	}

	// Save the object:
	data, err := d.marshalData(request.object)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		update %s set
			data = $1
		where
			id = $2
		returning
			creation_timestamp,
			deletion_timestamp
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, data, id)
	var (
		creationTs time.Time
		deletionTs time.Time
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
	)
	if err != nil {
		return
	}
	md := d.makeMetadata(creationTs, deletionTs)
	updated.SetId(id)
	updated.SetMetadata(md)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeUpdated,
		Object: updated,
	})

	response.object = updated
	return
}

// DeleteResponse contains the results of deleting an object.
type DeleteResponse[O Object] interface {
}

type deleteResponse[O Object] struct {
}

// DeleteRequest contains the parameters for deleting an object.
type DeleteRequest[O Object] interface {
	// SetObject sets the object to be deleted. This is equivalent to calling SetID with the idientifier of the
	// object.
	SetObject(O) DeleteRequest[O]

	// SetID sets the identifier of the object to be deleted.
	SetID(string) DeleteRequest[O]

	// Send sends the request and waits for the response.
	Send(context.Context) (DeleteResponse[O], error)
}

type deleteRequest[O Object] struct {
	d  *GenericDAO[O]
	id string
}

func (r *deleteRequest[O]) SetObject(value O) DeleteRequest[O] {
	r.id = value.GetId()
	return r
}

func (r *deleteRequest[O]) SetID(value string) DeleteRequest[O] {
	r.id = value
	return r
}

func (r *deleteRequest[O]) Send(ctx context.Context) (response DeleteResponse[O], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.delete(ctx, tx, r)
	return
}

// Delete deletes an object.
func (d *GenericDAO[O]) Delete() DeleteRequest[O] {
	return &deleteRequest[O]{
		d: d,
	}
}

func (d *GenericDAO[O]) delete(ctx context.Context, tx database.Tx, request *deleteRequest[O]) (response deleteResponse[O],
	err error) {
	if request.id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}

	// Set the deletion timestamp of the row and simultaneousyly retrieve the data, as we need it to fire the event
	// later:
	sql := fmt.Sprintf(
		`
		update %s set
			deletion_timestamp = now()
		where
			id = $1
		returning
			creation_timestamp,
			deletion_timestamp,
			data
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, request.id)
	var (
		creationTs time.Time
		deletionTs time.Time
		data       []byte
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&data,
	)
	if err != nil {
		return
	}
	deleted := d.newObject()
	err = d.unmarshalData(data, deleted)
	if err != nil {
		return
	}
	md := d.makeMetadata(creationTs, deletionTs)
	deleted.SetId(request.id)
	deleted.SetMetadata(md)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeDeleted,
		Object: deleted,
	})

	return
}

func (d *GenericDAO[O]) fireEvent(ctx context.Context, event Event) error {
	event.Table = d.table
	for _, eventCallback := range d.eventCallbacks {
		err := eventCallback(ctx, event)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *GenericDAO[O]) newId() string {
	return uuid.NewString()
}

func (d *GenericDAO[O]) newObject() O {
	return proto.Clone(d.objectTemplate).(O)
}

func (d *GenericDAO[O]) cloneObject(object O) O {
	return proto.Clone(object).(O)
}

func (d *GenericDAO[O]) marshalData(object O) (result []byte, err error) {
	// We need to marshal the object without the identifier and the metadata because those are stored in separate
	// columns.
	id := object.GetId()
	md := object.GetMetadata()
	object.SetId("")
	object.SetMetadata(nil)
	result, err = d.marshalOptions.Marshal(object)
	object.SetId(id)
	object.SetMetadata(md)
	return
}

func (d *GenericDAO[O]) unmarshalData(data []byte, object O) error {
	return protojson.Unmarshal(data, object)
}

func (d *GenericDAO[O]) makeMetadata(creationTimestamp, deletionTimestamp time.Time) *sharedv1.Metadata {
	result := &sharedv1.Metadata{}
	if creationTimestamp.Unix() != 0 {
		result.SetCreationTimestamp(timestamppb.New(creationTimestamp))
	}
	if deletionTimestamp.Unix() != 0 {
		result.SetDeletionTimestamp(timestamppb.New(deletionTimestamp))
	}
	return result
}
func isNil(object any) bool {
	return reflect.ValueOf(object).IsNil()
}

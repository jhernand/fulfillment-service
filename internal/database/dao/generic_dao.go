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

// PublicData is the interface that should be satisfied by types that contain the public data of an object.
type PublicData interface {
	proto.Message

	// GetId returns the unique identifier of the object.
	GetId() string

	// SetId sets the unique identifier of the object.
	SetId(string)

	// GetMetadata returns the public metadata of the object.
	GetMetadata() *sharedv1.Metadata

	// SetMetadata sets the public metadata of the object.
	SetMetadata(*sharedv1.Metadata)
}

// GenericDAOBuilder is a builder for creating generic data access objects.
type GenericDAOBuilder[Public PublicData] struct {
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
//   - `public_data` - The public data of the object, serialized object using the protocol buffers JSON serialization.
//
// Objects must have field named `id` of string type.
type GenericDAO[Public PublicData] struct {
	logger           *slog.Logger
	table            string
	defaultOrder     string
	defaultLimit     int
	maxLimit         int
	eventCallbacks   []EventCallback
	publicTemplate   Public
	marshalOptions   protojson.MarshalOptions
	filterTranslator *FilterTranslator[Public]
}

// NewGenericDAO creates a builder that can then be used to configure and create a generic DAO.
func NewGenericDAO[Public PublicData]() *GenericDAOBuilder[Public] {
	return &GenericDAOBuilder[Public]{
		defaultLimit: 100,
		maxLimit:     1000,
	}
}

// SetLogger sets the logger. This is mandatory.
func (b *GenericDAOBuilder[Public]) SetLogger(value *slog.Logger) *GenericDAOBuilder[Public] {
	b.logger = value
	return b
}

// SetTable sets the table name. This is mandatory.
func (b *GenericDAOBuilder[Public]) SetTable(value string) *GenericDAOBuilder[Public] {
	b.table = value
	return b
}

// SetDefaultOrder sets the default order criteria to use when nothing has been requested by the user. This is optional
// and the default is no order. This is intended only for use in unit tests, where it is convenient to have some
// predictable ordering.
func (b *GenericDAOBuilder[Public]) SetDefaultOrder(value string) *GenericDAOBuilder[Public] {
	b.defaultOrder = value
	return b
}

// SetDefaultLimit sets the default number of items returned. It will be used when the value of the limit parameter
// of the list request is zero. This is optional, and the default is 100.
func (b *GenericDAOBuilder[Public]) SetDefaultLimit(value int) *GenericDAOBuilder[Public] {
	b.defaultLimit = value
	return b
}

// SetMaxLimit sets the maximum number of items returned. This is optional and the default value is 1000.
func (b *GenericDAOBuilder[Public]) SetMaxLimit(value int) *GenericDAOBuilder[Public] {
	b.maxLimit = value
	return b
}

// AddEventCallback adds a function that will be called to process events when the DAO creates, updates or deletes
// an object.
//
// The functions are called synchronously, in the same order they were added, and with the same context used by the
// DAO for its operations. If any of them returns an error the transaction will be rolled back.
func (b *GenericDAOBuilder[Public]) AddEventCallback(value EventCallback) *GenericDAOBuilder[Public] {
	b.eventCallbacks = append(b.eventCallbacks, value)
	return b
}

// Build creates a new generic DAO using the configuration stored in the builder.
func (b *GenericDAOBuilder[Public]) Build() (result *GenericDAO[Public], err error) {
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

	// Create the template that we will clone when we need to create a new public:
	var public Public
	publicTemplate := public.ProtoReflect().New().Interface().(Public)

	// Prepare the JSON marshalling options:
	marshalOptions := protojson.MarshalOptions{
		UseProtoNames: true,
	}

	// Create the filter translator:
	filterTranslator, err := NewFilterTranslator[Public]().
		SetLogger(b.logger).
		Build()
	if err != nil {
		err = fmt.Errorf("failed to create filter translator: %w", err)
		return
	}

	// Create and populate the object:
	result = &GenericDAO[Public]{
		logger:           b.logger,
		table:            b.table,
		defaultOrder:     b.defaultOrder,
		defaultLimit:     b.defaultLimit,
		maxLimit:         b.maxLimit,
		eventCallbacks:   slices.Clone(b.eventCallbacks),
		publicTemplate:   publicTemplate,
		marshalOptions:   marshalOptions,
		filterTranslator: filterTranslator,
	}
	return
}

// ListResponse represents the result of a paginated query.
type ListResponse[Public PublicData] interface {
	// GetSize returns the actual number of items returned.
	GetSize() int

	// GetTotal returns the total number of items available.
	GetTotal() int

	// GetItems returns the list of items.
	GetItems() []Public
}

type listResponse[Public PublicData] struct {
	items []Public
	total int
}

func (r listResponse[Public]) GetSize() int {
	return len(r.items)
}

func (r listResponse[Public]) GetTotal() int {
	return r.total
}

func (r listResponse[Public]) GetItems() []Public {
	return r.items
}

// ListRequest represents the parameters for paginated queries.
type ListRequest[Public PublicData] interface {
	// SetOffset sets the starting point. This is optional, and the default is zero.
	SetOffset(int) ListRequest[Public]

	// SetLimit sets the maximum number of items. This is optional and the default is 100 unless a different value
	// is set using the SetDefaultLimit method of the builder of the DAO.
	SetLimit(int) ListRequest[Public]

	// SetFilter sets the CEL expression that defines which objects should be returned. This is optiona, and by
	// default it is empty, which means that all objects will be returned.
	SetFilter(string) ListRequest[Public]

	// Send sends the request and waits for the response.
	Send(context.Context) (ListResponse[Public], error)
}

type listRequest[Public PublicData] struct {
	d      *GenericDAO[Public]
	offset int
	limit  int
	filter string
}

func (r *listRequest[Public]) SetOffset(value int) ListRequest[Public] {
	r.offset = value
	return r
}

func (r *listRequest[Public]) SetLimit(value int) ListRequest[Public] {
	r.limit = value
	return r
}

func (r *listRequest[Public]) SetFilter(value string) ListRequest[Public] {
	r.filter = value
	return r
}

func (r *listRequest[Public]) Send(ctx context.Context) (response ListResponse[Public], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.list(ctx, tx, r)
	return
}

// List runs a query.
func (d *GenericDAO[Public]) List() ListRequest[Public] {
	return &listRequest[Public]{
		d: d,
	}
}

func (d *GenericDAO[Public]) list(ctx context.Context, tx database.Tx, request *listRequest[Public]) (response listResponse[Public],
	err error) {
	// Calculate the filter:
	var filter string
	if request.filter != "" {
		filter, err = d.filterTranslator.Translate(ctx, request.filter)
		if err != nil {
			return
		}
	}

	// Calculate the order clause:
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
			public_data
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
	var items []Public
	for itemsRows.Next() {
		var (
			id          string
			creationTs  time.Time
			deletionTs  time.Time
			publicBytes []byte
		)
		err = itemsRows.Scan(
			&id,
			&creationTs,
			&deletionTs,
			&publicBytes,
		)
		if err != nil {
			return
		}
		item := d.newPublic()
		err = d.unmarshalPublic(publicBytes, item)
		if err != nil {
			return
		}
		md := d.makePublicMetadata(creationTs, deletionTs)
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
type GetResponse[Public PublicData] interface {
	// GetExists returns true if the object exists, and false otherwise.
	GetExists() bool

	// GetPublic return the public data of the object, or nil if no such object exists.
	GetPublic() Public
}

type getResponse[Public PublicData] struct {
	exists bool
	public Public
}

func (r getResponse[Public]) GetExists() bool {
	return r.exists
}

func (r getResponse[Public]) GetPublic() Public {
	return r.public
}

// GetRequest contains the parameters to fetch a single object.
type GetRequest[Public PublicData] interface {
	// SetPublic sets the public data of the object to fetach. This is equivaent to calling SetID with the unique
	// identifier of the object.
	SetPublic(Public) GetRequest[Public]

	// SetID sets the identifier of the object.
	SetID(string) GetRequest[Public]

	// Send sends the request and waits for the response. Note that no error will be returned if the object doesn't
	// exist. Use the HasObject or GetObject methods of the response to check of the object exists.
	Send(context.Context) (GetResponse[Public], error)
}

type getRequest[Public PublicData] struct {
	d  *GenericDAO[Public]
	id string
}

func (r *getRequest[Public]) SetPublic(value Public) GetRequest[Public] {
	r.id = value.GetId()
	return r
}

func (r *getRequest[Public]) SetID(value string) GetRequest[Public] {
	r.id = value
	return r
}

func (r *getRequest[Public]) Send(ctx context.Context) (response GetResponse[Public], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.get(ctx, tx, r)
	return
}

// Get fetches a single object.
func (d *GenericDAO[Public]) Get() GetRequest[Public] {
	return &getRequest[Public]{
		d: d,
	}
}

func (d *GenericDAO[Public]) get(ctx context.Context, tx database.Tx, request *getRequest[Public]) (response getResponse[Public],
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
			public_data
		from
			%s
		where
			id = $1
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, query, request.id)
	var (
		creationTs  time.Time
		deletionTs  time.Time
		publicBytes []byte
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&publicBytes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
		return
	}
	public := d.newPublic()
	err = d.unmarshalPublic(publicBytes, public)
	if err != nil {
		return
	}
	md := d.makePublicMetadata(creationTs, deletionTs)
	public.SetId(request.id)
	public.SetMetadata(md)
	response.exists = true
	response.public = public
	return
}

// Exists checks if a row with the given identifiers exists. Returns false and no error if there is no row with the
// given identifier.
func (d *GenericDAO[Public]) Exists(ctx context.Context, id string) (ok bool, err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	ok, err = d.exists(ctx, tx, id)
	return
}

func (d *GenericDAO[Public]) exists(ctx context.Context, tx database.Tx, id string) (ok bool, err error) {
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
type CreateResponse[Public PublicData] interface {
	// GetPublic returns the public data of the created object.
	GetPublic() Public
}

type createResponse[Public PublicData] struct {
	public Public
}

func (r createResponse[Public]) GetPublic() Public {
	return r.public
}

type createRequest[Public PublicData] struct {
	d      *GenericDAO[Public]
	public Public
}

type CreateRequest[Public PublicData] interface {
	// SetPublic sets the initial public data of the object. This will not be modified. Use the GetPublic method of
	// the response to get the created object.
	SetPublic(public Public) CreateRequest[Public]

	// Send sends the request and waits for the response.
	Send(ctx context.Context) (CreateResponse[Public], error)
}

func (r *createRequest[Public]) SetPublic(public Public) CreateRequest[Public] {
	r.public = public
	return r
}

func (r *createRequest[Public]) Send(ctx context.Context) (response CreateResponse[Public], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	if isNil(r.public) {
		r.public = r.d.newPublic()
	}
	response, err = r.d.create(ctx, tx, r)
	return
}

// Create creates a new object.
func (d *GenericDAO[Public]) Create() CreateRequest[Public] {
	return &createRequest[Public]{
		d: d,
	}
}

func (d *GenericDAO[Public]) create(ctx context.Context, tx database.Tx, request *createRequest[Public]) (response createResponse[Public],
	err error) {
	// Generate an identifier if needed:
	id := request.public.GetId()
	if id == "" {
		id = d.newId()
	}

	// Save the object:
	publicBytes, err := d.marshalPublic(request.public)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		insert into %s (
			id,
			public_data
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
	row := tx.QueryRow(ctx, sql, id, publicBytes)
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
	public := d.clonePublic(request.public)
	publicMetadata := d.makePublicMetadata(creationTs, deletionTs)
	public.SetId(id)
	public.SetMetadata(publicMetadata)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeCreated,
		Object: public,
	})
	if err != nil {
		return
	}

	response.public = public
	return
}

// UpdateResponse response contains the result of updating an object.
type UpdateResponse[Public PublicData] interface {
	// GetPublic returns the public data of the object after the update.
	GetPublic() Public
}

type updateResponse[Public PublicData] struct {
	public Public
}

func (r updateResponse[Public]) GetPublic() Public {
	return r.public
}

// UpdateRequest contains the parameters for updating an object.
type UpdateRequest[Public PublicData] interface {
	// SetPublic sets the new public data of the object.
	SetPublic(Public) UpdateRequest[Public]

	// Send sends the request and waits for the response.
	Send(context.Context) (UpdateResponse[Public], error)
}

type updateRequest[Public PublicData] struct {
	d      *GenericDAO[Public]
	public Public
}

func (r *updateRequest[Public]) SetPublic(public Public) UpdateRequest[Public] {
	r.public = public
	return r
}

func (r *updateRequest[Public]) Send(ctx context.Context) (response UpdateResponse[Public], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.update(ctx, tx, r)
	return
}

// Update updates an existing object.
func (d *GenericDAO[Public]) Update() UpdateRequest[Public] {
	return &updateRequest[Public]{
		d: d,
	}
}

func (d *GenericDAO[Public]) update(ctx context.Context, tx database.Tx, request *updateRequest[Public]) (response updateResponse[Public],
	err error) {
	// Get the current object:
	id := request.public.GetId()
	if id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}
	getResponse, err := d.get(ctx, tx, &getRequest[Public]{
		id: id,
	})
	if err != nil {
		return
	}
	current := getResponse.public

	// Do nothing if there are no changes:
	updated := d.clonePublic(request.public)
	updated.SetMetadata(nil)
	current.SetMetadata(nil)
	if proto.Equal(updated, current) {
		return
	}

	// Save the object:
	publicBytes, err := d.marshalPublic(request.public)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		update %s set
			public_data = $1
		where
			id = $2
		returning
			creation_timestamp,
			deletion_timestamp
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, publicBytes, id)
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
	md := d.makePublicMetadata(creationTs, deletionTs)
	updated.SetId(id)
	updated.SetMetadata(md)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeUpdated,
		Object: updated,
	})

	response.public = updated
	return
}

// DeleteResponse contains the results of deleting an object.
type DeleteResponse[Public PublicData] interface {
}

type deleteResponse[Public PublicData] struct {
}

// DeleteRequest contains the parameters for deleting an object.
type DeleteRequest[Public PublicData] interface {
	// SetPublic sets the public data of the object to be deleted. This is equivalent to calling SetID with the
	// unique identifier of the object.
	SetPublic(Public) DeleteRequest[Public]

	// SetID sets the identifier of the object to be deleted.
	SetID(string) DeleteRequest[Public]

	// Send sends the request and waits for the response.
	Send(context.Context) (DeleteResponse[Public], error)
}

type deleteRequest[Public PublicData] struct {
	d  *GenericDAO[Public]
	id string
}

func (r *deleteRequest[Public]) SetPublic(value Public) DeleteRequest[Public] {
	r.id = value.GetId()
	return r
}

func (r *deleteRequest[Public]) SetID(value string) DeleteRequest[Public] {
	r.id = value
	return r
}

func (r *deleteRequest[Public]) Send(ctx context.Context) (response DeleteResponse[Public], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.delete(ctx, tx, r)
	return
}

// Delete deletes an object.
func (d *GenericDAO[Public]) Delete() DeleteRequest[Public] {
	return &deleteRequest[Public]{
		d: d,
	}
}

func (d *GenericDAO[Public]) delete(ctx context.Context, tx database.Tx, request *deleteRequest[Public]) (response deleteResponse[Public],
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
			public_data
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, request.id)
	var (
		creationTs  time.Time
		deletionTs  time.Time
		publicBytes []byte
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&publicBytes,
	)
	if err != nil {
		return
	}
	public := d.newPublic()
	err = d.unmarshalPublic(publicBytes, public)
	if err != nil {
		return
	}
	publicMetadata := d.makePublicMetadata(creationTs, deletionTs)
	public.SetId(request.id)
	public.SetMetadata(publicMetadata)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeDeleted,
		Object: public,
	})

	return
}

func (d *GenericDAO[Public]) fireEvent(ctx context.Context, event Event) error {
	event.Table = d.table
	for _, eventCallback := range d.eventCallbacks {
		err := eventCallback(ctx, event)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *GenericDAO[Public]) newId() string {
	return uuid.NewString()
}

func (d *GenericDAO[Public]) newPublic() Public {
	return proto.Clone(d.publicTemplate).(Public)
}

func (d *GenericDAO[Public]) clonePublic(public Public) Public {
	return proto.Clone(public).(Public)
}

func (d *GenericDAO[Public]) marshalPublic(public Public) (result []byte, err error) {
	// We need to marshal the object without the identifier and the metadata because those are stored in separate
	// columns.
	id := public.GetId()
	metadata := public.GetMetadata()
	public.SetId("")
	public.SetMetadata(nil)
	defer func() {
		public.SetId(id)
		public.SetMetadata(metadata)
	}()
	result, err = d.marshalOptions.Marshal(public)
	return
}

func (d *GenericDAO[Public]) unmarshalPublic(bytes []byte, public Public) error {
	return protojson.Unmarshal(bytes, public)
}

func (d *GenericDAO[Public]) makePublicMetadata(creationTimestamp, deletionTimestamp time.Time) *sharedv1.Metadata {
	result := &sharedv1.Metadata{}
	if creationTimestamp.Unix() != 0 {
		result.SetCreationTimestamp(timestamppb.New(creationTimestamp))
	}
	if deletionTimestamp.Unix() != 0 {
		result.SetDeletionTimestamp(timestamppb.New(deletionTimestamp))
	}
	return result
}

func isNil(value proto.Message) bool {
	return reflect.ValueOf(value).IsNil()
}

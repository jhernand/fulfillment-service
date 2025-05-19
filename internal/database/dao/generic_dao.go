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

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
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

// PrivateData is the interface that should be satisfied by types that contain the data data of an object.
type PrivateData interface {
	proto.Message

	// GetMetadata returns the private metadata of the object.
	GetMetadata() *privatev1.Metadata

	// SetMetadata sets the private metadata of the object.
	SetMetadata(*privatev1.Metadata)
}

// GenericDAOBuilder is a builder for creating generic data access objects.
type GenericDAOBuilder[Public PublicData, Private PrivateData] struct {
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
//   - `private_data` - The private data of the object, serialized object using the protocol buffers JSON serialization.
//
// Objects must have field named `id` of string type.
type GenericDAO[Public PublicData, Private PrivateData] struct {
	logger           *slog.Logger
	table            string
	defaultOrder     string
	defaultLimit     int
	maxLimit         int
	eventCallbacks   []EventCallback
	publicTemplate   Public
	privateTemplate  Private
	marshalOptions   protojson.MarshalOptions
	filterTranslator *FilterTranslator[Public]
}

// NewGenericDAO creates a builder that can then be used to configure and create a generic DAO.
func NewGenericDAO[Public PublicData, Private PrivateData]() *GenericDAOBuilder[Public, Private] {
	return &GenericDAOBuilder[Public, Private]{
		defaultLimit: 100,
		maxLimit:     1000,
	}
}

// SetLogger sets the logger. This is mandatory.
func (b *GenericDAOBuilder[Public, Private]) SetLogger(value *slog.Logger) *GenericDAOBuilder[Public, Private] {
	b.logger = value
	return b
}

// SetTable sets the table name. This is mandatory.
func (b *GenericDAOBuilder[Public, Private]) SetTable(value string) *GenericDAOBuilder[Public, Private] {
	b.table = value
	return b
}

// SetDefaultOrder sets the default order criteria to use when nothing has been requested by the user. This is optional
// and the default is no order. This is intended only for use in unit tests, where it is convenient to have some
// predictable ordering.
func (b *GenericDAOBuilder[Public, Private]) SetDefaultOrder(value string) *GenericDAOBuilder[Public, Private] {
	b.defaultOrder = value
	return b
}

// SetDefaultLimit sets the default number of items returned. It will be used when the value of the limit parameter
// of the list request is zero. This is optional, and the default is 100.
func (b *GenericDAOBuilder[Public, Private]) SetDefaultLimit(value int) *GenericDAOBuilder[Public, Private] {
	b.defaultLimit = value
	return b
}

// SetMaxLimit sets the maximum number of items returned. This is optional and the default value is 1000.
func (b *GenericDAOBuilder[Public, Private]) SetMaxLimit(value int) *GenericDAOBuilder[Public, Private] {
	b.maxLimit = value
	return b
}

// AddEventCallback adds a function that will be called to process events when the DAO creates, updates or deletes
// an object.
//
// The functions are called synchronously, in the same order they were added, and with the same context used by the
// DAO for its operations. If any of them returns an error the transaction will be rolled back.
func (b *GenericDAOBuilder[Public, Private]) AddEventCallback(value EventCallback) *GenericDAOBuilder[Public, Private] {
	b.eventCallbacks = append(b.eventCallbacks, value)
	return b
}

// Build creates a new generic DAO using the configuration stored in the builder.
func (b *GenericDAOBuilder[Public, Private]) Build() (result *GenericDAO[Public, Private], err error) {
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

	// Create the templates that we will clone when we need to create a new public or private data:
	var (
		public  Public
		private Private
	)
	publicTemplate := public.ProtoReflect().New().Interface().(Public)
	privateTemplate := private.ProtoReflect().New().Interface().(Private)

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
	result = &GenericDAO[Public, Private]{
		logger:           b.logger,
		table:            b.table,
		defaultOrder:     b.defaultOrder,
		defaultLimit:     b.defaultLimit,
		maxLimit:         b.maxLimit,
		eventCallbacks:   slices.Clone(b.eventCallbacks),
		publicTemplate:   publicTemplate,
		privateTemplate:  privateTemplate,
		marshalOptions:   marshalOptions,
		filterTranslator: filterTranslator,
	}
	return
}

// ListResponse represents the result of a paginated query.
type ListResponse[Public PublicData, Private PrivateData] interface {
	// GetSize returns the actual number of items returned.
	GetSize() int

	// GetTotal returns the total number of items available.
	GetTotal() int

	// GetPublic returns the list of public items.
	GetPublic() []Public

	// GetPrivate returns the list of private items.
	GetPrivate() []Private
}

type listResponse[Public PublicData, Private PrivateData] struct {
	size    int
	total   int
	public  []Public
	private []Private
}

func (r listResponse[Public, Private]) GetSize() int {
	return r.size
}

func (r listResponse[Public, Private]) GetTotal() int {
	return r.total
}

func (r listResponse[Public, Private]) GetPublic() []Public {
	return r.public
}

func (r listResponse[Public, Private]) GetPrivate() []Private {
	return r.private
}

// ListRequest represents the parameters for paginated queries.
type ListRequest[Public PublicData, Private PrivateData] interface {
	// SetOffset sets the starting point. This is optional, and the default is zero.
	SetOffset(int) ListRequest[Public, Private]

	// SetLimit sets the maximum number of items. This is optional and the default is 100 unless a different value
	// is set using the SetDefaultLimit method of the builder of the DAO.
	SetLimit(int) ListRequest[Public, Private]

	// SetFilter sets the CEL expression that defines which objects should be returned. This is optiona, and by
	// default it is empty, which means that all objects will be returned.
	SetFilter(string) ListRequest[Public, Private]

	// Send sends the request and waits for the response.
	Send(context.Context) (ListResponse[Public, Private], error)
}

type listRequest[Public PublicData, Private PrivateData] struct {
	d      *GenericDAO[Public, Private]
	offset int
	limit  int
	filter string
}

func (r *listRequest[Public, Private]) SetOffset(value int) ListRequest[Public, Private] {
	r.offset = value
	return r
}

func (r *listRequest[Public, Private]) SetLimit(value int) ListRequest[Public, Private] {
	r.limit = value
	return r
}

func (r *listRequest[Public, Private]) SetFilter(value string) ListRequest[Public, Private] {
	r.filter = value
	return r
}

func (r *listRequest[Public, Private]) Send(ctx context.Context) (response ListResponse[Public, Private], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.list(ctx, tx, r)
	return
}

// List runs a query.
func (d *GenericDAO[Public, Private]) List() ListRequest[Public, Private] {
	return &listRequest[Public, Private]{
		d: d,
	}
}

func (d *GenericDAO[Public, Private]) list(ctx context.Context, tx database.Tx, request *listRequest[Public, Private]) (response listResponse[Public, Private],
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
			public_data,
			private_data
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
	var (
		publicItems  []Public
		privateItems []Private
	)
	for itemsRows.Next() {
		var (
			id           string
			creationTs   time.Time
			deletionTs   time.Time
			publicBytes  []byte
			privateBytes []byte
		)
		err = itemsRows.Scan(
			&id,
			&creationTs,
			&deletionTs,
			&publicBytes,
			&privateBytes,
		)
		if err != nil {
			return
		}
		publicItem := d.newPublic()
		err = d.unmarshalPublic(publicBytes, publicItem)
		if err != nil {
			return
		}
		publicMetadata := d.makePublicMetadata(creationTs, deletionTs)
		publicItem.SetId(id)
		publicItem.SetMetadata(publicMetadata)
		publicItems = append(publicItems, publicItem)
		privateItem := d.newPrivate()
		err = d.unmarshalPrivate(privateBytes, privateItem)
		if err != nil {
			return
		}
		privateItems = append(privateItems, privateItem)
	}
	err = itemsRows.Err()
	if err != nil {
		return
	}
	response.size = len(publicItems)
	response.total = total
	response.public = publicItems
	response.private = privateItems
	return
}

// GetResponse contains the result of fetching a single object.
type GetResponse[Public PublicData, Private PrivateData] interface {
	// GetExists returns true if the object exists, and false otherwise.
	GetExists() bool

	// GetPublic return the public data of the object, or nil if no such object exists.
	GetPublic() Public

	// GetPriave return the private data of the object, or nil if no such object exists.
	GetPrivate() Private
}

type getResponse[Public PublicData, Private PrivateData] struct {
	exists  bool
	public  Public
	private Private
}

func (r getResponse[Public, Private]) GetExists() bool {
	return r.exists
}

func (r getResponse[Public, Private]) GetPublic() Public {
	return r.public
}

func (r getResponse[Public, Private]) GetPrivate() Private {
	return r.private
}

// GetRequest contains the parameters to fetch a single object.
type GetRequest[Public PublicData, Private PrivateData] interface {
	// SetPublic sets the public data of the object to fetach. This is equivaent to calling SetID with the unique
	// identifier of the object.
	SetPublic(Public) GetRequest[Public, Private]

	// SetID sets the identifier of the object.
	SetID(string) GetRequest[Public, Private]

	// Send sends the request and waits for the response. Note that no error will be returned if the object doesn't
	// exist. Use the HasObject or GetObject methods of the response to check of the object exists.
	Send(context.Context) (GetResponse[Public, Private], error)
}

type getRequest[Public PublicData, Private PrivateData] struct {
	d  *GenericDAO[Public, Private]
	id string
}

func (r *getRequest[Public, Private]) SetPublic(value Public) GetRequest[Public, Private] {
	r.id = value.GetId()
	return r
}

func (r *getRequest[Public, Private]) SetID(value string) GetRequest[Public, Private] {
	r.id = value
	return r
}

func (r *getRequest[Public, Private]) Send(ctx context.Context) (response GetResponse[Public, Private], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.get(ctx, tx, r)
	return
}

// Get fetches a single object.
func (d *GenericDAO[Public, Private]) Get() GetRequest[Public, Private] {
	return &getRequest[Public, Private]{
		d: d,
	}
}

func (d *GenericDAO[Public, Private]) get(ctx context.Context, tx database.Tx, request *getRequest[Public, Private]) (response getResponse[Public, Private],
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
			public_data,
			private_data
		from
			%s
		where
			id = $1
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, query, request.id)
	var (
		creationTs   time.Time
		deletionTs   time.Time
		publicBytes  []byte
		privateBytes []byte
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&publicBytes,
		&privateBytes,
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
	private := d.newPrivate()
	err = d.unmarshalPrivate(privateBytes, private)
	if err != nil {
		return
	}
	response.exists = true
	response.public = public
	response.private = private
	return
}

// Exists checks if a row with the given identifiers exists. Returns false and no error if there is no row with the
// given identifier.
func (d *GenericDAO[Public, Private]) Exists(ctx context.Context, id string) (ok bool, err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	ok, err = d.exists(ctx, tx, id)
	return
}

func (d *GenericDAO[Public, Private]) exists(ctx context.Context, tx database.Tx, id string) (ok bool, err error) {
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
type CreateResponse[Public PublicData, Private PrivateData] interface {
	// GetPublic returns the public data of the created object.
	GetPublic() Public

	// GetPrivate returns the private data of the created object.
	GetPrivate() Private
}

type createResponse[Public PublicData, Private PrivateData] struct {
	public  Public
	private Private
}

func (r createResponse[Public, Private]) GetPublic() Public {
	return r.public
}

func (r createResponse[Public, Private]) GetPrivate() Private {
	return r.private
}

type createRequest[Public PublicData, Private PrivateData] struct {
	d       *GenericDAO[Public, Private]
	public  Public
	private Private
}

type CreateRequest[Public PublicData, Private PrivateData] interface {
	// SetPublic sets the initial public data of the object. This will not be modified. Use the GetPublic method of
	// the response to get the public data of the created object.
	SetPublic(public Public) CreateRequest[Public, Private]

	// SetPrivate sets the initial public data of the object. This will not be modified. Use the GetPrivate method
	// of the response to get the private data of the created object.
	SetPrivate(private Private) CreateRequest[Public, Private]

	// Send sends the request and waits for the response.
	Send(ctx context.Context) (CreateResponse[Public, Private], error)
}

func (r *createRequest[Public, Private]) SetPublic(public Public) CreateRequest[Public, Private] {
	r.public = public
	return r
}

func (r *createRequest[Public, Private]) SetPrivate(private Private) CreateRequest[Public, Private] {
	r.private = private
	return r
}

func (r *createRequest[Public, Private]) Send(ctx context.Context) (response CreateResponse[Public, Private], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	if isNil(r.public) {
		r.public = r.d.newPublic()
	}
	if isNil(r.private) {
		r.private = r.d.newPrivate()
	}
	response, err = r.d.create(ctx, tx, r)
	return
}

// Create creates a new object.
func (d *GenericDAO[Public, Private]) Create() CreateRequest[Public, Private] {
	return &createRequest[Public, Private]{
		d: d,
	}
}

func (d *GenericDAO[Public, Private]) create(ctx context.Context, tx database.Tx, request *createRequest[Public, Private]) (response createResponse[Public, Private],
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
	privateBytes, err := d.marshalPrivate(request.private)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		insert into %s (
			id,
			public_data,
			private_data
		) values (
		 	$1,
			$2,
			$3
		)
		returning
			creation_timestamp,
			deletion_timestamp
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, id, publicBytes, privateBytes)
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
	private := d.clonePrivate(request.private)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeCreated,
		Object: public,
	})
	if err != nil {
		return
	}

	response.public = public
	response.private = private
	return
}

// UpdateResponse response contains the result of updating an object.
type UpdateResponse[Public PublicData, Private PrivateData] interface {
	// GetPublic returns the public data of the object after the update.
	GetPublic() Public

	// GetPrivate returns the private data of the object after the update.
	GetPrivate() Private
}

type updateResponse[Public PublicData, Private PrivateData] struct {
	public  Public
	private Private
}

func (r updateResponse[Public, Private]) GetPublic() Public {
	return r.public
}

func (r updateResponse[Public, Private]) GetPrivate() Private {
	return r.private
}

// UpdateRequest contains the parameters for updating an object.
type UpdateRequest[Public PublicData, Private PrivateData] interface {
	// SetPublic sets the new public data of the object.
	SetPublic(Public) UpdateRequest[Public, Private]

	// SetPrivate sets the new private data of the object.
	SetPrivate(Private) UpdateRequest[Public, Private]

	// Send sends the request and waits for the response.
	Send(context.Context) (UpdateResponse[Public, Private], error)
}

type updateRequest[Public PublicData, Private PrivateData] struct {
	d       *GenericDAO[Public, Private]
	public  Public
	private Private
}

func (r *updateRequest[Public, Private]) SetPublic(public Public) UpdateRequest[Public, Private] {
	r.public = public
	return r
}

func (r *updateRequest[Public, Private]) SetPrivate(private Private) UpdateRequest[Public, Private] {
	r.private = private
	return r
}

func (r *updateRequest[Public, Private]) Send(ctx context.Context) (response UpdateResponse[Public, Private], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.update(ctx, tx, r)
	return
}

// Update updates an existing object.
func (d *GenericDAO[Public, Private]) Update() UpdateRequest[Public, Private] {
	return &updateRequest[Public, Private]{
		d: d,
	}
}

func (d *GenericDAO[Public, Private]) update(ctx context.Context, tx database.Tx, request *updateRequest[Public, Private]) (response updateResponse[Public, Private],
	err error) {
	// Get the current object:
	id := request.public.GetId()
	if id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}
	getResponse, err := d.get(ctx, tx, &getRequest[Public, Private]{
		id: id,
	})
	if err != nil {
		return
	}
	currentPublic := getResponse.public
	currentPrivate := getResponse.private

	// Check if something has changed in the public data, but excluding the metadata, so we need to use a copy:
	updatedPublic := d.clonePublic(request.public)
	updatedPublic.SetMetadata(nil)
	currentPublic.SetMetadata(nil)
	publicChanged := !isNil(updatedPublic) && !proto.Equal(updatedPublic, currentPublic)

	// Check if something has changed in the private data:
	updatedPrivate := d.clonePrivate(request.private)
	updatedPrivate.SetMetadata(nil)
	currentPrivate.SetMetadata(nil)
	privateChanged := !isNil(updatedPrivate) && !proto.Equal(updatedPrivate, currentPrivate)

	// Do nothing if there are no changes:
	if !publicChanged && !privateChanged {
		return
	}

	// Save the object:
	publicBytes, err := d.marshalPublic(request.public)
	if err != nil {
		return
	}
	privateBytes, err := d.marshalPrivate(request.private)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		update %s set
			public_data = $1,
			private_data = $2
		where
			id = $3
		returning
			creation_timestamp,
			deletion_timestamp
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, publicBytes, privateBytes, id)
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
	publicMetadata := d.makePublicMetadata(creationTs, deletionTs)
	updatedPublic.SetId(id)
	updatedPublic.SetMetadata(publicMetadata)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeUpdated,
		Object: updatedPublic,
	})

	response.public = updatedPublic
	response.private = updatedPrivate
	return
}

// DeleteResponse contains the results of deleting an object.
type DeleteResponse[Public PublicData, Private PrivateData] interface {
}

type deleteResponse[Public PublicData, Private PrivateData] struct {
}

// DeleteRequest contains the parameters for deleting an object.
type DeleteRequest[Public PublicData, Private PrivateData] interface {
	// SetPublic sets the public data of the object to be deleted. This is equivalent to calling SetID with the
	// unique identifier of the object.
	SetPublic(Public) DeleteRequest[Public, Private]

	// SetID sets the identifier of the object to be deleted.
	SetID(string) DeleteRequest[Public, Private]

	// Send sends the request and waits for the response.
	Send(context.Context) (DeleteResponse[Public, Private], error)
}

type deleteRequest[Public PublicData, Private PrivateData] struct {
	d  *GenericDAO[Public, Private]
	id string
}

func (r *deleteRequest[Public, Private]) SetPublic(value Public) DeleteRequest[Public, Private] {
	r.id = value.GetId()
	return r
}

func (r *deleteRequest[Public, Private]) SetID(value string) DeleteRequest[Public, Private] {
	r.id = value
	return r
}

func (r *deleteRequest[Public, Private]) Send(ctx context.Context) (response DeleteResponse[Public, Private], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = r.d.delete(ctx, tx, r)
	return
}

// Delete deletes an object.
func (d *GenericDAO[Public, Private]) Delete() DeleteRequest[Public, Private] {
	return &deleteRequest[Public, Private]{
		d: d,
	}
}

func (d *GenericDAO[Public, Private]) delete(ctx context.Context, tx database.Tx, request *deleteRequest[Public, Private]) (response deleteResponse[Public, Private],
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

func (d *GenericDAO[Public, Private]) fireEvent(ctx context.Context, event Event) error {
	event.Table = d.table
	for _, eventCallback := range d.eventCallbacks {
		err := eventCallback(ctx, event)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *GenericDAO[Public, Private]) newId() string {
	return uuid.NewString()
}

func (d *GenericDAO[Public, Private]) newPublic() Public {
	return proto.Clone(d.publicTemplate).(Public)
}

func (d *GenericDAO[Public, Private]) newPrivate() Private {
	return proto.Clone(d.privateTemplate).(Private)
}

func (d *GenericDAO[Public, Private]) clonePublic(public Public) Public {
	return proto.Clone(public).(Public)
}

func (d *GenericDAO[Public, Private]) clonePrivate(private Private) Private {
	return proto.Clone(private).(Private)
}

func (d *GenericDAO[Public, Private]) marshalPublic(public Public) (result []byte, err error) {
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

func (d *GenericDAO[Public, Private]) marshalPrivate(private Private) (result []byte, err error) {
	result, err = d.marshalOptions.Marshal(private)
	return
}

func (d *GenericDAO[Public, Private]) unmarshalPublic(bytes []byte, public Public) error {
	return protojson.Unmarshal(bytes, public)
}

func (d *GenericDAO[Public, Private]) unmarshalPrivate(bytes []byte, private Private) error {
	return protojson.Unmarshal(bytes, private)
}

func (d *GenericDAO[Public, Private]) makePublicMetadata(creationTimestamp, deletionTimestamp time.Time) *sharedv1.Metadata {
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

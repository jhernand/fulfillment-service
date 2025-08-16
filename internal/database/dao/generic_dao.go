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
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/innabox/fulfillment-service/internal/database"
	"github.com/innabox/fulfillment-service/internal/json"
)

// Object is the interface that should be satisfied by objects to be managed by the generic DAO.
type Object interface {
	proto.Message
	GetId() string
	SetId(string)
}

// GenericDAOBuilder is a builder for creating generic data access objects.
type GenericDAOBuilder[O Object] struct {
	logger         *slog.Logger
	table          string
	defaultOrder   string
	defaultLimit   int32
	maxLimit       int32
	eventCallbacks []EventCallback
	ownershipFunc  func(ctx context.Context) []string
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
	defaultLimit     int32
	maxLimit         int32
	timestampDesc    protoreflect.MessageDescriptor
	eventCallbacks   []EventCallback
	objectTemplate   protoreflect.Message
	metadataField    protoreflect.FieldDescriptor
	metadataTemplate protoreflect.Message
	jsonEncoder      *json.Encoder
	marshalOptions   protojson.MarshalOptions
	unmarshalOptions protojson.UnmarshalOptions
	filterTranslator *FilterTranslator[O]
	ownershipFunc    func(ctx context.Context) []string
}

type metadataIface interface {
	proto.Message
	GetCreationTimestamp() *timestamppb.Timestamp
	SetCreationTimestamp(*timestamppb.Timestamp)
	GetDeletionTimestamp() *timestamppb.Timestamp
	SetDeletionTimestamp(*timestamppb.Timestamp)
	GetFinalizers() []string
	SetFinalizers([]string)
	GetOwners() []string
	SetOwners([]string)
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
	b.defaultLimit = int32(value)
	return b
}

// SetMaxLimit sets the maximum number of items returned. This is optional and the default value is 1000.
func (b *GenericDAOBuilder[O]) SetMaxLimit(value int) *GenericDAOBuilder[O] {
	b.maxLimit = int32(value)
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

// SetOwnershipFunc sets the function that will be used to determine the owners value for objects. The function receives
// the context as a parameter and should return the names of the owner. If not provided, no owner will be set.
func (b *GenericDAOBuilder[O]) SetOwnershipFunc(value func(ctx context.Context) []string) *GenericDAOBuilder[O] {
	b.ownershipFunc = value
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

	// Get descriptors of well known types:
	var timestamp *timestamppb.Timestamp
	timestampDesc := timestamp.ProtoReflect().Descriptor()

	// Create the template that we will clone when we need to create a new object:
	var object O
	objectTemplate := object.ProtoReflect()

	// Get the field descriptors:
	objectDesc := objectTemplate.Descriptor()
	objectFields := objectDesc.Fields()
	idField := objectFields.ByName(idFieldName)
	if idField == nil {
		err = fmt.Errorf(
			"object of type '%s' doesn't have a '%s' field",
			objectDesc.FullName(), idFieldName,
		)
		return
	}
	metadataField := objectFields.ByName(metadataFieldName)
	if metadataField == nil {
		err = fmt.Errorf(
			"object of type '%s' doesn't have a '%s' field",
			objectDesc.FullName(), metadataFieldName,
		)
		return
	}

	// Create the template that we will clone when we need to create a new metadata object:
	metadataTemplate := objectTemplate.NewField(metadataField).Message()

	// Create the JSON encoder. We need this special encoder in order to ignore the 'id' and 'metadata' fields
	// because we save those in separate database columns and not in the JSON document where we save everything
	// else.
	jsonEncoder, err := json.NewEncoder().
		SetLogger(b.logger).
		AddIgnoredFields(
			idField.FullName(),
			metadataField.FullName(),
		).
		Build()
	if err != nil {
		err = fmt.Errorf("failed to create JSON encoder: %w", err)
		return
	}

	// Prepare the JSON marshalling options:
	marshalOptions := protojson.MarshalOptions{
		UseProtoNames: true,
	}
	unmarshalOptions := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}

	// Create the filter translator:
	filterTranslator, err := NewFilterTranslator[O]().
		SetLogger(b.logger).
		Build()
	if err != nil {
		err = fmt.Errorf("failed to create filter translator: %w", err)
		return
	}

	// Set the default ownership function so that it will never be nil:
	ownershipFunc := b.ownershipFunc
	if ownershipFunc == nil {
		ownershipFunc = unknownOwnershipFunc
	}

	// Create and populate the object:
	result = &GenericDAO[O]{
		logger:           b.logger,
		table:            b.table,
		defaultOrder:     b.defaultOrder,
		defaultLimit:     b.defaultLimit,
		maxLimit:         b.maxLimit,
		timestampDesc:    timestampDesc,
		eventCallbacks:   slices.Clone(b.eventCallbacks),
		objectTemplate:   objectTemplate,
		metadataField:    metadataField,
		metadataTemplate: metadataTemplate,
		jsonEncoder:      jsonEncoder,
		marshalOptions:   marshalOptions,
		unmarshalOptions: unmarshalOptions,
		filterTranslator: filterTranslator,
		ownershipFunc:    ownershipFunc,
	}
	return
}

// ListRequest represents the parameters for paginated queries.
type ListRequest struct {
	// Offset specifies the starting point.
	Offset int32

	// Limit specifies the maximum number of items.
	Limit int32

	// Filter is the CEL expression that defines which objects should be returned.
	Filter string
}

// ListResponse represents the result of a paginated query.
type ListResponse[I any] struct {
	// Size is the actual number of items returned.
	Size int32

	// Total is the total number of items available.
	Total int32

	// Items is the list of items.
	Items []I
}

// List retrieves all rows from the table and deserializes them into a slice of messages.
func (d *GenericDAO[O]) List(ctx context.Context, request ListRequest) (response ListResponse[O], err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	response, err = d.list(ctx, tx, request)
	return
}

func (d *GenericDAO[O]) list(ctx context.Context, tx database.Tx, request ListRequest) (response ListResponse[O],
	err error) {
	// Calculate the filter:
	var filter string
	if request.Filter != "" {
		filter, err = d.filterTranslator.Translate(ctx, request.Filter)
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
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}

	// Calculate the limit:
	limit := request.Limit
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
			finalizers,
			owners,
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
			finalizers []string
			owners     []string
			data       []byte
		)
		err = itemsRows.Scan(
			&id,
			&creationTs,
			&deletionTs,
			&finalizers,
			&owners,
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
		md := d.makeMetadata(creationTs, deletionTs, finalizers, owners)
		item.SetId(id)
		d.setMetadata(item, md)
		items = append(items, item)
	}
	err = itemsRows.Err()
	if err != nil {
		return
	}
	response.Size = int32(len(items))
	response.Total = int32(total)
	response.Items = items
	return
}

// Get retrieves a single row by its identifier and deserializes it into a message. Returns nil and no error if there
// is no row with the given identifier.
func (d *GenericDAO[O]) Get(ctx context.Context, id string) (result O, err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	result, err = d.get(ctx, tx, id)
	return
}

func (d *GenericDAO[O]) get(ctx context.Context, tx database.Tx, id string) (result O, err error) {
	if id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}
	sql := fmt.Sprintf(
		`
		select
			creation_timestamp,
			deletion_timestamp,
			finalizers,
			owners,
			data
		from
			%s
		where
			id = $1
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, id)
	var (
		creationTs time.Time
		deletionTs time.Time
		finalizers []string
		owners     []string
		data       []byte
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&finalizers,
		&owners,
		&data,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
		return
	}
	if err != nil {
		return
	}
	object := d.newObject()
	err = d.unmarshalData(data, object)
	if err != nil {
		return
	}
	metadata := d.makeMetadata(creationTs, deletionTs, finalizers, owners)
	object.SetId(id)
	d.setMetadata(object, metadata)
	result = object
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

// Create adds a new row to the table with a generated identifier and serialized data.
func (d *GenericDAO[O]) Create(ctx context.Context, object O) (result O, err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	result, err = d.create(ctx, tx, object)
	return
}

func (d *GenericDAO[O]) create(ctx context.Context, tx database.Tx, object O) (result O, err error) {
	// Generate an identifier if needed:
	id := object.GetId()
	if id == "" {
		id = d.newId()
	}

	// Get the metadata:
	metadata := d.getMetadata(object)
	finalizers := d.getFinalizers(metadata)
	owners := d.ownershipFunc(ctx)
	if owners == nil {
		owners = []string{}
	}

	// Save the object:
	data, err := d.marshalData(object)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		insert into %s (
			id,
			finalizers,
			owners,
			data
		) values (
		 	$1,
		 	$2,
			$3,
			$4
		)
		returning
			creation_timestamp,
			deletion_timestamp
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, id, finalizers, owners, data)
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
	created := d.cloneObject(object)
	metadata = d.makeMetadata(creationTs, deletionTs, finalizers, owners)
	created.SetId(id)
	d.setMetadata(created, metadata)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeCreated,
		Object: created,
	})
	if err != nil {
		return
	}

	result = created
	return
}

// Update modifies an existing row in the table by its identifier with the result of serializing the provided object.
func (d *GenericDAO[O]) Update(ctx context.Context, object O) (result O, err error) {
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	result, err = d.update(ctx, tx, object)
	return
}

func (d *GenericDAO[O]) update(ctx context.Context, tx database.Tx, object O) (result O, err error) {
	// Get the current object:
	id := object.GetId()
	if id == "" {
		err = errors.New("object identifier is mandatory")
		return
	}
	current, err := d.get(ctx, tx, id)
	if err != nil {
		return
	}

	// Do nothing if there are no changes:
	if d.equivalent(current, object) {
		return
	}

	// Get the metadata:
	metadata := d.getMetadata(object)
	finalizers := d.getFinalizers(metadata)

	// Save the object:
	data, err := d.marshalData(object)
	if err != nil {
		return
	}
	sql := fmt.Sprintf(
		`
		update %s set
			finalizers = $1,
			data = $2
		where
			id = $3
		returning
			creation_timestamp,
			deletion_timestamp,
			owners
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, finalizers, data, id)
	var (
		creationTs time.Time
		deletionTs time.Time
		owners     []string
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&owners,
	)
	if err != nil {
		return
	}
	object = d.cloneObject(object)
	metadata = d.makeMetadata(creationTs, deletionTs, finalizers, owners)
	object.SetId(id)
	d.setMetadata(object, metadata)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeUpdated,
		Object: object,
	})
	if err != nil {
		return
	}

	// If the object has been deleted and there are no finalizers we can now archive the object and delete the row:
	if deletionTs.Unix() != 0 && len(finalizers) == 0 {
		err = d.archive(ctx, tx, id, creationTs, deletionTs, owners, data)
		if err != nil {
			return
		}
	}

	// Return the updated object:
	result = object
	return
}

// Delete removes a row from the table by its identifier.
func (d *GenericDAO[O]) Delete(ctx context.Context, id string) (err error) {
	// Start a transaction:
	tx, err := database.TxFromContext(ctx)
	if err != nil {
		return
	}
	defer tx.ReportError(&err)
	err = d.delete(ctx, tx, id)
	return
}

func (d *GenericDAO[O]) delete(ctx context.Context, tx database.Tx, id string) (err error) {
	if id == "" {
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
			finalizers,
			owners,
			data
		`,
		d.table,
	)
	row := tx.QueryRow(ctx, sql, id)
	var (
		creationTs time.Time
		deletionTs time.Time
		finalizers []string
		owners     []string
		data       []byte
	)
	err = row.Scan(
		&creationTs,
		&deletionTs,
		&finalizers,
		&owners,
		&data,
	)
	if err != nil {
		return
	}
	object := d.newObject()
	err = d.unmarshalData(data, object)
	if err != nil {
		return
	}
	metadata := d.makeMetadata(creationTs, deletionTs, finalizers, owners)
	object.SetId(id)
	d.setMetadata(object, metadata)

	// Fire the event:
	err = d.fireEvent(ctx, Event{
		Type:   EventTypeDeleted,
		Object: object,
	})
	if err != nil {
		return err
	}

	// If there are no finalizers we can now archive the object and delete the row:
	if len(finalizers) == 0 {
		err = d.archive(ctx, tx, id, creationTs, deletionTs, owners, data)
		if err != nil {
			return
		}
	}

	return
}

func (d *GenericDAO[O]) archive(ctx context.Context, tx database.Tx, id string, creationTs, deletionTs time.Time,
	owners []string, data []byte) error {
	sql := fmt.Sprintf(
		`
		insert into archived_%s (
			id,
			creation_timestamp,
			deletion_timestamp,
			owners,
			data
		) values (
		 	$1,
			$2,
			$3,
			$4,
			$5
		)
		`,
		d.table,
	)
	_, err := tx.Exec(ctx, sql, id, creationTs, deletionTs, owners, data)
	if err != nil {
		return err
	}
	sql = fmt.Sprintf(`delete from %s where id = $1`, d.table)
	_, err = tx.Exec(ctx, sql, id)
	return err
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
	return d.objectTemplate.New().Interface().(O)
}

func (d *GenericDAO[O]) cloneObject(object O) O {
	return proto.Clone(object).(O)
}

func (d *GenericDAO[O]) marshalData(object O) (result []byte, err error) {
	result, err = d.jsonEncoder.Marshal(object)
	return
}

func (d *GenericDAO[O]) unmarshalData(data []byte, object O) error {
	return d.unmarshalOptions.Unmarshal(data, object)
}

func (d *GenericDAO[O]) makeMetadata(creationTs, deletionTs time.Time, finalizers []string,
	owners []string) metadataIface {
	result := d.metadataTemplate.New().Interface().(metadataIface)
	if creationTs.Unix() != 0 {
		result.SetCreationTimestamp(timestamppb.New(creationTs))
	}
	if deletionTs.Unix() != 0 {
		result.SetDeletionTimestamp(timestamppb.New(deletionTs))
	}
	result.SetFinalizers(finalizers)
	result.SetOwners(owners)
	return result
}

func (d *GenericDAO[O]) getMetadata(object O) metadataIface {
	objectReflect := object.ProtoReflect()
	if !objectReflect.Has(d.metadataField) {
		return nil
	}
	return objectReflect.Get(d.metadataField).Message().Interface().(metadataIface)
}

func (d *GenericDAO[O]) setMetadata(object O, metadata metadataIface) {
	objectReflect := object.ProtoReflect()
	if metadata != nil {
		metadataReflect := metadata.ProtoReflect()
		objectReflect.Set(d.metadataField, protoreflect.ValueOfMessage(metadataReflect))
	} else {
		objectReflect.Clear(d.metadataField)
	}
}

func (d *GenericDAO[O]) getFinalizers(metadata metadataIface) []string {
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

// equivalent checks if two objects are equivalent. That means that they are equal excepty maybe in the creation and
// deletion timestamps.
func (d *GenericDAO[O]) equivalent(x, y O) bool {
	return d.equivalentMessages(x.ProtoReflect(), y.ProtoReflect())
}

func (d *GenericDAO[O]) equivalentMessages(x, y protoreflect.Message) bool {
	if x.IsValid() != y.IsValid() {
		return false
	}
	fields := x.Descriptor().Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		xPresent := x.Has(field)
		yPresent := y.Has(field)
		if xPresent != yPresent {
			return false
		}
		if !xPresent && !yPresent {
			continue
		}
		xValue := x.Get(field)
		yValue := y.Get(field)
		switch field.Name() {
		case metadataFieldName:
			if !d.equivalentMetadata(xValue.Message(), yValue.Message()) {
				return false
			}
		default:
			if !xValue.Equal(yValue) {
				return false
			}
		}
	}
	return true
}

func (d *GenericDAO[O]) equivalentMetadata(x, y protoreflect.Message) bool {
	if x.IsValid() != y.IsValid() {
		return false
	}
	fields := x.Descriptor().Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if field.Name() == creationTimestampFieldName || field.Name() == deletionTimestampFieldName {
			continue
		}
		xv := x.Get(field)
		yv := y.Get(field)
		if !xv.Equal(yv) {
			return false
		}
	}
	return true
}

// unknownOwnershipFunc is a function that returns an empty list of owners.
func unknownOwnershipFunc(_ context.Context) []string {
	return nil
}

// Names of well known fields:
var (
	creationTimestampFieldName = protoreflect.Name("creation_timestamp")
	deletionTimestampFieldName = protoreflect.Name("deletion_timestamp")
	idFieldName                = protoreflect.Name("id")
	metadataFieldName          = protoreflect.Name("metadata")
)

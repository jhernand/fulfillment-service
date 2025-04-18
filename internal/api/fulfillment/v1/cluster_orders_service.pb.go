//
// Copyright (c) 2025 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        (unknown)
// source: fulfillment/v1/cluster_orders_service.proto

package fulfillmentv1

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ClusterOrdersListRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Index of the first result. If not specified the default value will be zero.
	Offset *int32 `protobuf:"varint,1,opt,name=offset,proto3,oneof" json:"offset,omitempty"`
	// Maximum number of results to be returned by the server. When not specified all the results will be returned. Note
	// that there may not be enough results to return, and that the server may decide, for performance reasons, to return
	// less results than requested.
	Limit *int32 `protobuf:"varint,2,opt,name=limit,proto3,oneof" json:"limit,omitempty"`
	// Filter criteria.
	//
	// The syntax of this parameter is similar to the syntax of the _where_ clause of a SQL statement, but using the names
	// of the attributes of the order instead of the names of the columns of a table. For example, in order to retrieve
	// all the orders with state `FULFILLED` the value should be:
	//
	//	state = 'FULLFILLED'
	//
	// If this isn't provided, or if the value is empty, then all the orders that the user has permission to see will be
	// returned.
	Filter *string `protobuf:"bytes,3,opt,name=filter,proto3,oneof" json:"filter,omitempty"`
	// Order criteria.
	//
	// The syntax of this parameter is similar to the syntax of the _order by_ clause of a SQL statement, but using the
	// names of the attributes of the order instead of the names of the columns of a table. For example, in order to sort
	// the orders descending by state the value should be:
	//
	//	state desc
	//
	// If the parameter isn't provided, or if the value is empty, then the order of the results is undefined.
	Order         *string `protobuf:"bytes,4,opt,name=order,proto3,oneof" json:"order,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersListRequest) Reset() {
	*x = ClusterOrdersListRequest{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersListRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersListRequest) ProtoMessage() {}

func (x *ClusterOrdersListRequest) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersListRequest.ProtoReflect.Descriptor instead.
func (*ClusterOrdersListRequest) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{0}
}

func (x *ClusterOrdersListRequest) GetOffset() int32 {
	if x != nil && x.Offset != nil {
		return *x.Offset
	}
	return 0
}

func (x *ClusterOrdersListRequest) GetLimit() int32 {
	if x != nil && x.Limit != nil {
		return *x.Limit
	}
	return 0
}

func (x *ClusterOrdersListRequest) GetFilter() string {
	if x != nil && x.Filter != nil {
		return *x.Filter
	}
	return ""
}

func (x *ClusterOrdersListRequest) GetOrder() string {
	if x != nil && x.Order != nil {
		return *x.Order
	}
	return ""
}

type ClusterOrdersListResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Actual number of items returned. Note that this may be smaller than the value requested in the `limit` parameter
	// of the request if there are not enough items, or of the system decides that returning that number of items isn't
	// feasible or convenient for performance reasons.
	Size *int32 `protobuf:"varint,3,opt,name=size,proto3,oneof" json:"size,omitempty"`
	// Total number of items of the collection that match the search criteria, regardless of the number of results
	// requested with the `limit` parameter.
	Total *int32 `protobuf:"varint,4,opt,name=total,proto3,oneof" json:"total,omitempty"`
	// List of results.
	Items         []*ClusterOrder `protobuf:"bytes,5,rep,name=items,proto3" json:"items,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersListResponse) Reset() {
	*x = ClusterOrdersListResponse{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersListResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersListResponse) ProtoMessage() {}

func (x *ClusterOrdersListResponse) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersListResponse.ProtoReflect.Descriptor instead.
func (*ClusterOrdersListResponse) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{1}
}

func (x *ClusterOrdersListResponse) GetSize() int32 {
	if x != nil && x.Size != nil {
		return *x.Size
	}
	return 0
}

func (x *ClusterOrdersListResponse) GetTotal() int32 {
	if x != nil && x.Total != nil {
		return *x.Total
	}
	return 0
}

func (x *ClusterOrdersListResponse) GetItems() []*ClusterOrder {
	if x != nil {
		return x.Items
	}
	return nil
}

type ClusterOrdersGetRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersGetRequest) Reset() {
	*x = ClusterOrdersGetRequest{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersGetRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersGetRequest) ProtoMessage() {}

func (x *ClusterOrdersGetRequest) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersGetRequest.ProtoReflect.Descriptor instead.
func (*ClusterOrdersGetRequest) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{2}
}

func (x *ClusterOrdersGetRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

type ClusterOrdersGetResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Object        *ClusterOrder          `protobuf:"bytes,1,opt,name=object,proto3" json:"object,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersGetResponse) Reset() {
	*x = ClusterOrdersGetResponse{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersGetResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersGetResponse) ProtoMessage() {}

func (x *ClusterOrdersGetResponse) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersGetResponse.ProtoReflect.Descriptor instead.
func (*ClusterOrdersGetResponse) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{3}
}

func (x *ClusterOrdersGetResponse) GetObject() *ClusterOrder {
	if x != nil {
		return x.Object
	}
	return nil
}

type ClusterOrdersCreateRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Object        *ClusterOrder          `protobuf:"bytes,1,opt,name=object,proto3" json:"object,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersCreateRequest) Reset() {
	*x = ClusterOrdersCreateRequest{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersCreateRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersCreateRequest) ProtoMessage() {}

func (x *ClusterOrdersCreateRequest) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersCreateRequest.ProtoReflect.Descriptor instead.
func (*ClusterOrdersCreateRequest) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{4}
}

func (x *ClusterOrdersCreateRequest) GetObject() *ClusterOrder {
	if x != nil {
		return x.Object
	}
	return nil
}

type ClusterOrdersCreateResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Object        *ClusterOrder          `protobuf:"bytes,1,opt,name=object,proto3" json:"object,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersCreateResponse) Reset() {
	*x = ClusterOrdersCreateResponse{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersCreateResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersCreateResponse) ProtoMessage() {}

func (x *ClusterOrdersCreateResponse) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersCreateResponse.ProtoReflect.Descriptor instead.
func (*ClusterOrdersCreateResponse) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{5}
}

func (x *ClusterOrdersCreateResponse) GetObject() *ClusterOrder {
	if x != nil {
		return x.Object
	}
	return nil
}

type ClusterOrdersDeleteRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersDeleteRequest) Reset() {
	*x = ClusterOrdersDeleteRequest{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersDeleteRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersDeleteRequest) ProtoMessage() {}

func (x *ClusterOrdersDeleteRequest) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersDeleteRequest.ProtoReflect.Descriptor instead.
func (*ClusterOrdersDeleteRequest) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{6}
}

func (x *ClusterOrdersDeleteRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

type ClusterOrdersDeleteResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersDeleteResponse) Reset() {
	*x = ClusterOrdersDeleteResponse{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersDeleteResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersDeleteResponse) ProtoMessage() {}

func (x *ClusterOrdersDeleteResponse) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersDeleteResponse.ProtoReflect.Descriptor instead.
func (*ClusterOrdersDeleteResponse) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{7}
}

type ClusterOrdersUpdateStatusRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Status        *ClusterOrderStatus    `protobuf:"bytes,2,opt,name=status,proto3" json:"status,omitempty"`
	UpdateMask    *fieldmaskpb.FieldMask `protobuf:"bytes,3,opt,name=update_mask,json=updateMask,proto3" json:"update_mask,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersUpdateStatusRequest) Reset() {
	*x = ClusterOrdersUpdateStatusRequest{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersUpdateStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersUpdateStatusRequest) ProtoMessage() {}

func (x *ClusterOrdersUpdateStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersUpdateStatusRequest.ProtoReflect.Descriptor instead.
func (*ClusterOrdersUpdateStatusRequest) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{8}
}

func (x *ClusterOrdersUpdateStatusRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *ClusterOrdersUpdateStatusRequest) GetStatus() *ClusterOrderStatus {
	if x != nil {
		return x.Status
	}
	return nil
}

func (x *ClusterOrdersUpdateStatusRequest) GetUpdateMask() *fieldmaskpb.FieldMask {
	if x != nil {
		return x.UpdateMask
	}
	return nil
}

type ClusterOrdersUpdateStatusResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Status        *ClusterOrderStatus    `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrdersUpdateStatusResponse) Reset() {
	*x = ClusterOrdersUpdateStatusResponse{}
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[9]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrdersUpdateStatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrdersUpdateStatusResponse) ProtoMessage() {}

func (x *ClusterOrdersUpdateStatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_orders_service_proto_msgTypes[9]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrdersUpdateStatusResponse.ProtoReflect.Descriptor instead.
func (*ClusterOrdersUpdateStatusResponse) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP(), []int{9}
}

func (x *ClusterOrdersUpdateStatusResponse) GetStatus() *ClusterOrderStatus {
	if x != nil {
		return x.Status
	}
	return nil
}

var File_fulfillment_v1_cluster_orders_service_proto protoreflect.FileDescriptor

var file_fulfillment_v1_cluster_orders_service_proto_rawDesc = string([]byte{
	0x0a, 0x2b, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2f, 0x76, 0x31,
	0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x5f,
	0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0e, 0x66,
	0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x1a, 0x27, 0x66,
	0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x5f, 0x74, 0x79, 0x70, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61,
	0x70, 0x69, 0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x5f, 0x6d, 0x61, 0x73, 0x6b,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xb4, 0x01, 0x0a, 0x18, 0x43, 0x6c, 0x75, 0x73, 0x74,
	0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x1b, 0x0a, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x05, 0x48, 0x00, 0x52, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74, 0x88, 0x01, 0x01,
	0x12, 0x19, 0x0a, 0x05, 0x6c, 0x69, 0x6d, 0x69, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x48,
	0x01, 0x52, 0x05, 0x6c, 0x69, 0x6d, 0x69, 0x74, 0x88, 0x01, 0x01, 0x12, 0x1b, 0x0a, 0x06, 0x66,
	0x69, 0x6c, 0x74, 0x65, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x48, 0x02, 0x52, 0x06, 0x66,
	0x69, 0x6c, 0x74, 0x65, 0x72, 0x88, 0x01, 0x01, 0x12, 0x19, 0x0a, 0x05, 0x6f, 0x72, 0x64, 0x65,
	0x72, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x48, 0x03, 0x52, 0x05, 0x6f, 0x72, 0x64, 0x65, 0x72,
	0x88, 0x01, 0x01, 0x42, 0x09, 0x0a, 0x07, 0x5f, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74, 0x42, 0x08,
	0x0a, 0x06, 0x5f, 0x6c, 0x69, 0x6d, 0x69, 0x74, 0x42, 0x09, 0x0a, 0x07, 0x5f, 0x66, 0x69, 0x6c,
	0x74, 0x65, 0x72, 0x42, 0x08, 0x0a, 0x06, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x22, 0x96, 0x01,
	0x0a, 0x19, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x4c,
	0x69, 0x73, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x17, 0x0a, 0x04, 0x73,
	0x69, 0x7a, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x48, 0x00, 0x52, 0x04, 0x73, 0x69, 0x7a,
	0x65, 0x88, 0x01, 0x01, 0x12, 0x19, 0x0a, 0x05, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x18, 0x04, 0x20,
	0x01, 0x28, 0x05, 0x48, 0x01, 0x52, 0x05, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x88, 0x01, 0x01, 0x12,
	0x32, 0x0a, 0x05, 0x69, 0x74, 0x65, 0x6d, 0x73, 0x18, 0x05, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1c,
	0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e,
	0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x52, 0x05, 0x69, 0x74,
	0x65, 0x6d, 0x73, 0x42, 0x07, 0x0a, 0x05, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x42, 0x08, 0x0a, 0x06,
	0x5f, 0x74, 0x6f, 0x74, 0x61, 0x6c, 0x22, 0x29, 0x0a, 0x17, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65,
	0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x47, 0x65, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69,
	0x64, 0x22, 0x50, 0x0a, 0x18, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65,
	0x72, 0x73, 0x47, 0x65, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x34, 0x0a,
	0x06, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e,
	0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43,
	0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x52, 0x06, 0x6f, 0x62, 0x6a,
	0x65, 0x63, 0x74, 0x22, 0x52, 0x0a, 0x1a, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72,
	0x64, 0x65, 0x72, 0x73, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x34, 0x0a, 0x06, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1c, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e,
	0x76, 0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x52,
	0x06, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x22, 0x53, 0x0a, 0x1b, 0x43, 0x6c, 0x75, 0x73, 0x74,
	0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x34, 0x0a, 0x06, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c,
	0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f,
	0x72, 0x64, 0x65, 0x72, 0x52, 0x06, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x22, 0x2c, 0x0a, 0x1a,
	0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x44, 0x65, 0x6c,
	0x65, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x22, 0x1d, 0x0a, 0x1b, 0x43, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x44, 0x65, 0x6c, 0x65, 0x74,
	0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0xab, 0x01, 0x0a, 0x20, 0x43, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x55, 0x70, 0x64, 0x61, 0x74,
	0x65, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e,
	0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x3a,
	0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x22,
	0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e,
	0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x3b, 0x0a, 0x0b, 0x75, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x5f, 0x6d, 0x61, 0x73, 0x6b, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x46, 0x69, 0x65, 0x6c, 0x64, 0x4d, 0x61, 0x73, 0x6b, 0x52, 0x0a, 0x75, 0x70, 0x64,
	0x61, 0x74, 0x65, 0x4d, 0x61, 0x73, 0x6b, 0x22, 0x5f, 0x0a, 0x21, 0x43, 0x6c, 0x75, 0x73, 0x74,
	0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x53, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3a, 0x0a, 0x06,
	0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x22, 0x2e, 0x66,
	0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x32, 0xa0, 0x06, 0x0a, 0x0d, 0x43, 0x6c, 0x75,
	0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x12, 0x87, 0x01, 0x0a, 0x04, 0x4c,
	0x69, 0x73, 0x74, 0x12, 0x28, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e,
	0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65,
	0x72, 0x73, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x29, 0x2e,
	0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43,
	0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x4c, 0x69, 0x73, 0x74,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x2a, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x24,
	0x12, 0x22, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65,
	0x6e, 0x74, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x6f, 0x72,
	0x64, 0x65, 0x72, 0x73, 0x12, 0x91, 0x01, 0x0a, 0x03, 0x47, 0x65, 0x74, 0x12, 0x27, 0x2e, 0x66,
	0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x47, 0x65, 0x74, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x28, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d,
	0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72,
	0x64, 0x65, 0x72, 0x73, 0x47, 0x65, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x37, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x31, 0x62, 0x06, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x12,
	0x27, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e,
	0x74, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x6f, 0x72, 0x64,
	0x65, 0x72, 0x73, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0x9d, 0x01, 0x0a, 0x06, 0x43, 0x72, 0x65,
	0x61, 0x74, 0x65, 0x12, 0x2a, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e,
	0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65,
	0x72, 0x73, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x2b, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31,
	0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x43, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x3a, 0x82, 0xd3,
	0xe4, 0x93, 0x02, 0x34, 0x3a, 0x06, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x62, 0x06, 0x6f, 0x62,
	0x6a, 0x65, 0x63, 0x74, 0x22, 0x22, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69,
	0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65,
	0x72, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x12, 0x92, 0x01, 0x0a, 0x06, 0x44, 0x65, 0x6c,
	0x65, 0x74, 0x65, 0x12, 0x2a, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e,
	0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65,
	0x72, 0x73, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x2b, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31,
	0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x44, 0x65,
	0x6c, 0x65, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x2f, 0x82, 0xd3,
	0xe4, 0x93, 0x02, 0x29, 0x2a, 0x27, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69,
	0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65,
	0x72, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0xbb, 0x01,
	0x0a, 0x0c, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x30,
	0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e,
	0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x55, 0x70, 0x64,
	0x61, 0x74, 0x65, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x31, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76,
	0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x55,
	0x70, 0x64, 0x61, 0x74, 0x65, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x46, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x40, 0x3a, 0x06, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x62, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x32, 0x2e, 0x2f, 0x61, 0x70,
	0x69, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2f, 0x76, 0x31,
	0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x73, 0x2f,
	0x7b, 0x69, 0x64, 0x7d, 0x2f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x42, 0xda, 0x01, 0x0a, 0x12,
	0x63, 0x6f, 0x6d, 0x2e, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e,
	0x76, 0x31, 0x42, 0x19, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72,
	0x73, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a,
	0x50, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x69, 0x6e, 0x6e, 0x61,
	0x62, 0x6f, 0x78, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2d,
	0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74,
	0x2f, 0x76, 0x31, 0x3b, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x76,
	0x31, 0xa2, 0x02, 0x03, 0x46, 0x58, 0x58, 0xaa, 0x02, 0x0e, 0x46, 0x75, 0x6c, 0x66, 0x69, 0x6c,
	0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x0e, 0x46, 0x75, 0x6c, 0x66, 0x69,
	0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x1a, 0x46, 0x75, 0x6c, 0x66,
	0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65,
	0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x0f, 0x46, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c,
	0x6d, 0x65, 0x6e, 0x74, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_fulfillment_v1_cluster_orders_service_proto_rawDescOnce sync.Once
	file_fulfillment_v1_cluster_orders_service_proto_rawDescData []byte
)

func file_fulfillment_v1_cluster_orders_service_proto_rawDescGZIP() []byte {
	file_fulfillment_v1_cluster_orders_service_proto_rawDescOnce.Do(func() {
		file_fulfillment_v1_cluster_orders_service_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_fulfillment_v1_cluster_orders_service_proto_rawDesc), len(file_fulfillment_v1_cluster_orders_service_proto_rawDesc)))
	})
	return file_fulfillment_v1_cluster_orders_service_proto_rawDescData
}

var file_fulfillment_v1_cluster_orders_service_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_fulfillment_v1_cluster_orders_service_proto_goTypes = []any{
	(*ClusterOrdersListRequest)(nil),          // 0: fulfillment.v1.ClusterOrdersListRequest
	(*ClusterOrdersListResponse)(nil),         // 1: fulfillment.v1.ClusterOrdersListResponse
	(*ClusterOrdersGetRequest)(nil),           // 2: fulfillment.v1.ClusterOrdersGetRequest
	(*ClusterOrdersGetResponse)(nil),          // 3: fulfillment.v1.ClusterOrdersGetResponse
	(*ClusterOrdersCreateRequest)(nil),        // 4: fulfillment.v1.ClusterOrdersCreateRequest
	(*ClusterOrdersCreateResponse)(nil),       // 5: fulfillment.v1.ClusterOrdersCreateResponse
	(*ClusterOrdersDeleteRequest)(nil),        // 6: fulfillment.v1.ClusterOrdersDeleteRequest
	(*ClusterOrdersDeleteResponse)(nil),       // 7: fulfillment.v1.ClusterOrdersDeleteResponse
	(*ClusterOrdersUpdateStatusRequest)(nil),  // 8: fulfillment.v1.ClusterOrdersUpdateStatusRequest
	(*ClusterOrdersUpdateStatusResponse)(nil), // 9: fulfillment.v1.ClusterOrdersUpdateStatusResponse
	(*ClusterOrder)(nil),                      // 10: fulfillment.v1.ClusterOrder
	(*ClusterOrderStatus)(nil),                // 11: fulfillment.v1.ClusterOrderStatus
	(*fieldmaskpb.FieldMask)(nil),             // 12: google.protobuf.FieldMask
}
var file_fulfillment_v1_cluster_orders_service_proto_depIdxs = []int32{
	10, // 0: fulfillment.v1.ClusterOrdersListResponse.items:type_name -> fulfillment.v1.ClusterOrder
	10, // 1: fulfillment.v1.ClusterOrdersGetResponse.object:type_name -> fulfillment.v1.ClusterOrder
	10, // 2: fulfillment.v1.ClusterOrdersCreateRequest.object:type_name -> fulfillment.v1.ClusterOrder
	10, // 3: fulfillment.v1.ClusterOrdersCreateResponse.object:type_name -> fulfillment.v1.ClusterOrder
	11, // 4: fulfillment.v1.ClusterOrdersUpdateStatusRequest.status:type_name -> fulfillment.v1.ClusterOrderStatus
	12, // 5: fulfillment.v1.ClusterOrdersUpdateStatusRequest.update_mask:type_name -> google.protobuf.FieldMask
	11, // 6: fulfillment.v1.ClusterOrdersUpdateStatusResponse.status:type_name -> fulfillment.v1.ClusterOrderStatus
	0,  // 7: fulfillment.v1.ClusterOrders.List:input_type -> fulfillment.v1.ClusterOrdersListRequest
	2,  // 8: fulfillment.v1.ClusterOrders.Get:input_type -> fulfillment.v1.ClusterOrdersGetRequest
	4,  // 9: fulfillment.v1.ClusterOrders.Create:input_type -> fulfillment.v1.ClusterOrdersCreateRequest
	6,  // 10: fulfillment.v1.ClusterOrders.Delete:input_type -> fulfillment.v1.ClusterOrdersDeleteRequest
	8,  // 11: fulfillment.v1.ClusterOrders.UpdateStatus:input_type -> fulfillment.v1.ClusterOrdersUpdateStatusRequest
	1,  // 12: fulfillment.v1.ClusterOrders.List:output_type -> fulfillment.v1.ClusterOrdersListResponse
	3,  // 13: fulfillment.v1.ClusterOrders.Get:output_type -> fulfillment.v1.ClusterOrdersGetResponse
	5,  // 14: fulfillment.v1.ClusterOrders.Create:output_type -> fulfillment.v1.ClusterOrdersCreateResponse
	7,  // 15: fulfillment.v1.ClusterOrders.Delete:output_type -> fulfillment.v1.ClusterOrdersDeleteResponse
	9,  // 16: fulfillment.v1.ClusterOrders.UpdateStatus:output_type -> fulfillment.v1.ClusterOrdersUpdateStatusResponse
	12, // [12:17] is the sub-list for method output_type
	7,  // [7:12] is the sub-list for method input_type
	7,  // [7:7] is the sub-list for extension type_name
	7,  // [7:7] is the sub-list for extension extendee
	0,  // [0:7] is the sub-list for field type_name
}

func init() { file_fulfillment_v1_cluster_orders_service_proto_init() }
func file_fulfillment_v1_cluster_orders_service_proto_init() {
	if File_fulfillment_v1_cluster_orders_service_proto != nil {
		return
	}
	file_fulfillment_v1_cluster_order_type_proto_init()
	file_fulfillment_v1_cluster_orders_service_proto_msgTypes[0].OneofWrappers = []any{}
	file_fulfillment_v1_cluster_orders_service_proto_msgTypes[1].OneofWrappers = []any{}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_fulfillment_v1_cluster_orders_service_proto_rawDesc), len(file_fulfillment_v1_cluster_orders_service_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_fulfillment_v1_cluster_orders_service_proto_goTypes,
		DependencyIndexes: file_fulfillment_v1_cluster_orders_service_proto_depIdxs,
		MessageInfos:      file_fulfillment_v1_cluster_orders_service_proto_msgTypes,
	}.Build()
	File_fulfillment_v1_cluster_orders_service_proto = out.File
	file_fulfillment_v1_cluster_orders_service_proto_goTypes = nil
	file_fulfillment_v1_cluster_orders_service_proto_depIdxs = nil
}

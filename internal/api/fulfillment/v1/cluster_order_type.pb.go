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
// 	protoc-gen-go v1.36.4
// 	protoc        (unknown)
// source: fulfillment/v1/cluster_order_type.proto

package fulfillmentv1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
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

type ClusterOrder_State int32

const (
	// Unspecified indicates that the value isn't set.
	ClusterOrder_UNSPECIFIED ClusterOrder_State = 0
	// Accepted indicates that the order has been accepted by the system.
	ClusterOrder_ACCEPTED ClusterOrder_State = 1
	// Rejected indicates that the order has been rejected by the system.
	ClusterOrder_REJECTED ClusterOrder_State = 2
	// Rejected indicates that the order has been canceled by the user.
	ClusterOrder_CANCELED ClusterOrder_State = 3
	// Fulfilled indicates that the order has been successfully fulfilled.
	ClusterOrder_FULFILLED ClusterOrder_State = 4
	// Failed indicates that fulfillment of the order failed.
	ClusterOrder_FAILED ClusterOrder_State = 5
)

// Enum value maps for ClusterOrder_State.
var (
	ClusterOrder_State_name = map[int32]string{
		0: "UNSPECIFIED",
		1: "ACCEPTED",
		2: "REJECTED",
		3: "CANCELED",
		4: "FULFILLED",
		5: "FAILED",
	}
	ClusterOrder_State_value = map[string]int32{
		"UNSPECIFIED": 0,
		"ACCEPTED":    1,
		"REJECTED":    2,
		"CANCELED":    3,
		"FULFILLED":   4,
		"FAILED":      5,
	}
)

func (x ClusterOrder_State) Enum() *ClusterOrder_State {
	p := new(ClusterOrder_State)
	*p = x
	return p
}

func (x ClusterOrder_State) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (ClusterOrder_State) Descriptor() protoreflect.EnumDescriptor {
	return file_fulfillment_v1_cluster_order_type_proto_enumTypes[0].Descriptor()
}

func (ClusterOrder_State) Type() protoreflect.EnumType {
	return &file_fulfillment_v1_cluster_order_type_proto_enumTypes[0]
}

func (x ClusterOrder_State) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use ClusterOrder_State.Descriptor instead.
func (ClusterOrder_State) EnumDescriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_order_type_proto_rawDescGZIP(), []int{0, 0}
}

// A cluster order contains all the details that the user has to provide to request the provisioning of a cluster, as
// well as the details of the status of the fulfillment process.
type ClusterOrder struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Unique identifier of the order. This will be automatically generated by the system when the order is placed.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// Reference to the cluster template. This is mandatory, and must be the value of the `id` field of one of the cluster
	// templates.
	TemplateId string `protobuf:"bytes,2,opt,name=template_id,json=templateId,proto3" json:"template_id,omitempty"`
	// State indicates the current state of the processing of the order.
	State ClusterOrder_State `protobuf:"varint,4,opt,name=state,proto3,enum=fulfillment.v1.ClusterOrder_State" json:"state,omitempty"`
	// Reference to the resulting cluster. This will be automatically populated by the system when the requested cluster
	// is completely provisoned. Further details about the cluster, like the API URL, will be available there.
	ClusterId     string `protobuf:"bytes,3,opt,name=cluster_id,json=clusterId,proto3" json:"cluster_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrder) Reset() {
	*x = ClusterOrder{}
	mi := &file_fulfillment_v1_cluster_order_type_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrder) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrder) ProtoMessage() {}

func (x *ClusterOrder) ProtoReflect() protoreflect.Message {
	mi := &file_fulfillment_v1_cluster_order_type_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterOrder.ProtoReflect.Descriptor instead.
func (*ClusterOrder) Descriptor() ([]byte, []int) {
	return file_fulfillment_v1_cluster_order_type_proto_rawDescGZIP(), []int{0}
}

func (x *ClusterOrder) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *ClusterOrder) GetTemplateId() string {
	if x != nil {
		return x.TemplateId
	}
	return ""
}

func (x *ClusterOrder) GetState() ClusterOrder_State {
	if x != nil {
		return x.State
	}
	return ClusterOrder_UNSPECIFIED
}

func (x *ClusterOrder) GetClusterId() string {
	if x != nil {
		return x.ClusterId
	}
	return ""
}

var File_fulfillment_v1_cluster_order_type_proto protoreflect.FileDescriptor

var file_fulfillment_v1_cluster_order_type_proto_rawDesc = string([]byte{
	0x0a, 0x27, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2f, 0x76, 0x31,
	0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x5f, 0x74,
	0x79, 0x70, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0e, 0x66, 0x75, 0x6c, 0x66, 0x69,
	0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x22, 0xf7, 0x01, 0x0a, 0x0c, 0x43, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x1f, 0x0a, 0x0b, 0x74, 0x65,
	0x6d, 0x70, 0x6c, 0x61, 0x74, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0a, 0x74, 0x65, 0x6d, 0x70, 0x6c, 0x61, 0x74, 0x65, 0x49, 0x64, 0x12, 0x38, 0x0a, 0x05, 0x73,
	0x74, 0x61, 0x74, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x22, 0x2e, 0x66, 0x75, 0x6c,
	0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c, 0x75, 0x73,
	0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x05,
	0x73, 0x74, 0x61, 0x74, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72,
	0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x63, 0x6c, 0x75, 0x73, 0x74,
	0x65, 0x72, 0x49, 0x64, 0x22, 0x5d, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x0f, 0x0a,
	0x0b, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x0c,
	0x0a, 0x08, 0x41, 0x43, 0x43, 0x45, 0x50, 0x54, 0x45, 0x44, 0x10, 0x01, 0x12, 0x0c, 0x0a, 0x08,
	0x52, 0x45, 0x4a, 0x45, 0x43, 0x54, 0x45, 0x44, 0x10, 0x02, 0x12, 0x0c, 0x0a, 0x08, 0x43, 0x41,
	0x4e, 0x43, 0x45, 0x4c, 0x45, 0x44, 0x10, 0x03, 0x12, 0x0d, 0x0a, 0x09, 0x46, 0x55, 0x4c, 0x46,
	0x49, 0x4c, 0x4c, 0x45, 0x44, 0x10, 0x04, 0x12, 0x0a, 0x0a, 0x06, 0x46, 0x41, 0x49, 0x4c, 0x45,
	0x44, 0x10, 0x05, 0x42, 0xd6, 0x01, 0x0a, 0x12, 0x63, 0x6f, 0x6d, 0x2e, 0x66, 0x75, 0x6c, 0x66,
	0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x76, 0x31, 0x42, 0x15, 0x43, 0x6c, 0x75, 0x73,
	0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x54, 0x79, 0x70, 0x65, 0x50, 0x72, 0x6f, 0x74,
	0x6f, 0x50, 0x01, 0x5a, 0x50, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f,
	0x69, 0x6e, 0x6e, 0x61, 0x62, 0x6f, 0x78, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d,
	0x65, 0x6e, 0x74, 0x2d, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c,
	0x6d, 0x65, 0x6e, 0x74, 0x2f, 0x76, 0x31, 0x3b, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d,
	0x65, 0x6e, 0x74, 0x76, 0x31, 0xa2, 0x02, 0x03, 0x46, 0x58, 0x58, 0xaa, 0x02, 0x0e, 0x46, 0x75,
	0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x0e, 0x46,
	0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x1a,
	0x46, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x5c, 0x56, 0x31, 0x5c, 0x47,
	0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x0f, 0x46, 0x75, 0x6c,
	0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_fulfillment_v1_cluster_order_type_proto_rawDescOnce sync.Once
	file_fulfillment_v1_cluster_order_type_proto_rawDescData []byte
)

func file_fulfillment_v1_cluster_order_type_proto_rawDescGZIP() []byte {
	file_fulfillment_v1_cluster_order_type_proto_rawDescOnce.Do(func() {
		file_fulfillment_v1_cluster_order_type_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_fulfillment_v1_cluster_order_type_proto_rawDesc), len(file_fulfillment_v1_cluster_order_type_proto_rawDesc)))
	})
	return file_fulfillment_v1_cluster_order_type_proto_rawDescData
}

var file_fulfillment_v1_cluster_order_type_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_fulfillment_v1_cluster_order_type_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_fulfillment_v1_cluster_order_type_proto_goTypes = []any{
	(ClusterOrder_State)(0), // 0: fulfillment.v1.ClusterOrder.State
	(*ClusterOrder)(nil),    // 1: fulfillment.v1.ClusterOrder
}
var file_fulfillment_v1_cluster_order_type_proto_depIdxs = []int32{
	0, // 0: fulfillment.v1.ClusterOrder.state:type_name -> fulfillment.v1.ClusterOrder.State
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_fulfillment_v1_cluster_order_type_proto_init() }
func file_fulfillment_v1_cluster_order_type_proto_init() {
	if File_fulfillment_v1_cluster_order_type_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_fulfillment_v1_cluster_order_type_proto_rawDesc), len(file_fulfillment_v1_cluster_order_type_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_fulfillment_v1_cluster_order_type_proto_goTypes,
		DependencyIndexes: file_fulfillment_v1_cluster_order_type_proto_depIdxs,
		EnumInfos:         file_fulfillment_v1_cluster_order_type_proto_enumTypes,
		MessageInfos:      file_fulfillment_v1_cluster_order_type_proto_msgTypes,
	}.Build()
	File_fulfillment_v1_cluster_order_type_proto = out.File
	file_fulfillment_v1_cluster_order_type_proto_goTypes = nil
	file_fulfillment_v1_cluster_order_type_proto_depIdxs = nil
}

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
// source: admin/v1/cluster_order_type.proto

package adminv1

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

// Contains the details about order that are available only for the system.
type ClusterOrder struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Unique identifier of the order.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// Identifier of the hub that was selected for this order.
	HubId         string `protobuf:"bytes,2,opt,name=hub_id,json=hubId,proto3" json:"hub_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterOrder) Reset() {
	*x = ClusterOrder{}
	mi := &file_admin_v1_cluster_order_type_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterOrder) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterOrder) ProtoMessage() {}

func (x *ClusterOrder) ProtoReflect() protoreflect.Message {
	mi := &file_admin_v1_cluster_order_type_proto_msgTypes[0]
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
	return file_admin_v1_cluster_order_type_proto_rawDescGZIP(), []int{0}
}

func (x *ClusterOrder) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *ClusterOrder) GetHubId() string {
	if x != nil {
		return x.HubId
	}
	return ""
}

var File_admin_v1_cluster_order_type_proto protoreflect.FileDescriptor

var file_admin_v1_cluster_order_type_proto_rawDesc = string([]byte{
	0x0a, 0x21, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74,
	0x65, 0x72, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x12, 0x08, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x76, 0x31, 0x22, 0x35, 0x0a,
	0x0c, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x12, 0x0e, 0x0a,
	0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x15, 0x0a,
	0x06, 0x68, 0x75, 0x62, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x68,
	0x75, 0x62, 0x49, 0x64, 0x42, 0xac, 0x01, 0x0a, 0x0c, 0x63, 0x6f, 0x6d, 0x2e, 0x61, 0x64, 0x6d,
	0x69, 0x6e, 0x2e, 0x76, 0x31, 0x42, 0x15, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x4f, 0x72,
	0x64, 0x65, 0x72, 0x54, 0x79, 0x70, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x44,
	0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x69, 0x6e, 0x6e, 0x61, 0x62,
	0x6f, 0x78, 0x2f, 0x66, 0x75, 0x6c, 0x66, 0x69, 0x6c, 0x6c, 0x6d, 0x65, 0x6e, 0x74, 0x2d, 0x73,
	0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f,
	0x61, 0x70, 0x69, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f, 0x76, 0x31, 0x3b, 0x61, 0x64, 0x6d,
	0x69, 0x6e, 0x76, 0x31, 0xa2, 0x02, 0x03, 0x41, 0x58, 0x58, 0xaa, 0x02, 0x08, 0x41, 0x64, 0x6d,
	0x69, 0x6e, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x08, 0x41, 0x64, 0x6d, 0x69, 0x6e, 0x5c, 0x56, 0x31,
	0xe2, 0x02, 0x14, 0x41, 0x64, 0x6d, 0x69, 0x6e, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x09, 0x41, 0x64, 0x6d, 0x69, 0x6e, 0x3a,
	0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_admin_v1_cluster_order_type_proto_rawDescOnce sync.Once
	file_admin_v1_cluster_order_type_proto_rawDescData []byte
)

func file_admin_v1_cluster_order_type_proto_rawDescGZIP() []byte {
	file_admin_v1_cluster_order_type_proto_rawDescOnce.Do(func() {
		file_admin_v1_cluster_order_type_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_admin_v1_cluster_order_type_proto_rawDesc), len(file_admin_v1_cluster_order_type_proto_rawDesc)))
	})
	return file_admin_v1_cluster_order_type_proto_rawDescData
}

var file_admin_v1_cluster_order_type_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_admin_v1_cluster_order_type_proto_goTypes = []any{
	(*ClusterOrder)(nil), // 0: admin.v1.ClusterOrder
}
var file_admin_v1_cluster_order_type_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_admin_v1_cluster_order_type_proto_init() }
func file_admin_v1_cluster_order_type_proto_init() {
	if File_admin_v1_cluster_order_type_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_admin_v1_cluster_order_type_proto_rawDesc), len(file_admin_v1_cluster_order_type_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_admin_v1_cluster_order_type_proto_goTypes,
		DependencyIndexes: file_admin_v1_cluster_order_type_proto_depIdxs,
		MessageInfos:      file_admin_v1_cluster_order_type_proto_msgTypes,
	}.Build()
	File_admin_v1_cluster_order_type_proto = out.File
	file_admin_v1_cluster_order_type_proto_goTypes = nil
	file_admin_v1_cluster_order_type_proto_depIdxs = nil
}

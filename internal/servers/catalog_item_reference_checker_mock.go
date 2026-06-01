/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package servers

import (
	"context"
	"reflect"

	"go.uber.org/mock/gomock"
)

type MockCatalogItemReferenceChecker struct {
	ctrl     *gomock.Controller
	recorder *MockCatalogItemReferenceCheckerMockRecorder
	isgomock struct{}
}

type MockCatalogItemReferenceCheckerMockRecorder struct {
	mock *MockCatalogItemReferenceChecker
}

func NewMockCatalogItemReferenceChecker(ctrl *gomock.Controller) *MockCatalogItemReferenceChecker {
	mock := &MockCatalogItemReferenceChecker{ctrl: ctrl}
	mock.recorder = &MockCatalogItemReferenceCheckerMockRecorder{mock}
	return mock
}

func (m *MockCatalogItemReferenceChecker) EXPECT() *MockCatalogItemReferenceCheckerMockRecorder {
	return m.recorder
}

func (m *MockCatalogItemReferenceChecker) hasReference(ctx context.Context, catalogItemID string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "hasReference", ctx, catalogItemID)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockCatalogItemReferenceCheckerMockRecorder) hasReference(ctx, catalogItemID any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock, "hasReference",
		reflect.TypeOf((*MockCatalogItemReferenceChecker)(nil).hasReference),
		ctx, catalogItemID,
	)
}

/*
Copyright (c) 2026 Red Hat Inc.

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
	"errors"
	"fmt"
	"log/slog"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/osac-project/fulfillment-service/internal/annotations"
	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/database/dao"
)

// TemplateParameterDefinition captures the methods needed from a template parameter definition. Both cluster and
// compute instance template parameter definitions satisfy it.
type TemplateParameterDefinition interface {
	proto.Message

	GetName() string
	GetTitle() string
	SetTitle(string)
	GetDescription() string
	SetDescription(string)
	GetSealed() bool
	GetDefault() *anypb.Any
	SetDefault(*anypb.Any)
	SetSealed(bool)
}

// Template captures the methods needed from a template. Both cluster and compute instance templates satisfy it.
type Template[P TemplateParameterDefinition] interface {
	dao.Object

	HasParent() bool
	GetParent() string
	SetParent(string)
	GetParameters() []P
	SetParameters([]P)
	GetMetadata() *privatev1.Metadata
}

// TemplateHelperBuilder contains the data and logic needed to create a new template helper.
type TemplateHelperBuilder[T Template[P], P TemplateParameterDefinition] struct {
	logger *slog.Logger
	dao    *dao.GenericDAO[T]
}

// TemplateHelper provides methods for working with template inheritance: validating parent references on creation,
// computing the effective (flattened) view of a template, and resolving the Ansible role.
type TemplateHelper[T Template[P], P TemplateParameterDefinition] struct {
	logger *slog.Logger
	dao    *dao.GenericDAO[T]
}

// NewTemplateHelper creates a builder that can then be used to configure and create a new TemplateHelper.
func NewTemplateHelper[T Template[P], P TemplateParameterDefinition]() *TemplateHelperBuilder[T, P] {
	return &TemplateHelperBuilder[T, P]{}
}

// SetLogger sets the logger. This is mandatory.
func (b *TemplateHelperBuilder[T, P]) SetLogger(value *slog.Logger) *TemplateHelperBuilder[T, P] {
	b.logger = value
	return b
}

// SetDao sets the DAO used to look up templates. This is mandatory.
func (b *TemplateHelperBuilder[T, P]) SetDao(value *dao.GenericDAO[T]) *TemplateHelperBuilder[T, P] {
	b.dao = value
	return b
}

// Build uses the configuration stored in the builder to create a new template helper.
func (b *TemplateHelperBuilder[T, P]) Build() (result *TemplateHelper[T, P], err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.dao == nil {
		err = errors.New("DAO is mandatory")
		return
	}

	// Create and populate the object:
	result = &TemplateHelper[T, P]{
		logger: b.logger,
		dao:    b.dao,
	}
	return
}

// ValidateCreation validates the parent reference and parameters of a template being created. It checks that the
// parent exists and is visible, that the child parameters reference valid parent parameters (recursively), and that
// sealed parameters are not being unsealed.
func (h *TemplateHelper[T, P]) ValidateCreation(ctx context.Context, template T) error {
	return h.validateStructure(ctx, template)
}

// Normalize replaces the parent reference with the parent's identifier, in case the caller used the name instead.
// This is a no-op if the template has no parent or if the parent is already set to the identifier.
func (h *TemplateHelper[T, P]) Normalize(ctx context.Context, template T) error {
	if !template.HasParent() {
		return nil
	}
	parentRef := template.GetParent()
	if parentRef == "" {
		return nil
	}
	parent, err := h.lookupTemplate(ctx, parentRef)
	if err != nil {
		return err
	}
	template.SetParent(parent.GetId())
	return nil
}

// ValidateUpdate validates that an update to an existing template doesn't violate the inheritance rules. It checks
// the template's own parent/parameter structure and additionally verifies that child templates are not broken by
// the change (i.e. every child parameter still references a valid, non-sealed effective parameter of this template).
func (h *TemplateHelper[T, P]) ValidateUpdate(ctx context.Context, template T) error {
	err := h.validateStructure(ctx, template)
	if err != nil {
		return err
	}

	// Calculate what the flattened parmaeters of this template will be after the update:
	parameterList, err := h.calculateFlattenedParameters(ctx, template)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Failed to calculate flattened parameters",
			slog.String("template", template.GetId()),
			slog.Any("error", err),
		)
		return grpcstatus.Errorf(
			grpccodes.Internal,
			"failed to calculate flattened parameters for template '%s'",
			template.GetId(),
		)
	}

	// Create an index of parameters by name, so that lookups will be falster in the rest of the method:
	parameterIndex := make(map[string]P, len(parameterList))
	for _, parameter := range parameterList {
		parameterIndex[parameter.GetName()] = parameter
	}

	// Find all templates that reference this one as their parent:
	children, err := h.dao.List().
		SetFilter(fmt.Sprintf("this.parent == %q", template.GetId())).
		Do(ctx)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Failed to list child templates",
			slog.String("template", template.GetId()),
			slog.Any("error", err),
		)
		return grpcstatus.Errorf(grpccodes.Internal, "failed to list child templates")
	}

	// Verify that every child parameter still references a valid effective parameter:
	for _, child := range children.GetItems() {
		for _, childParam := range child.GetParameters() {
			parameter, exists := parameterIndex[childParam.GetName()]
			if !exists {
				return grpcstatus.Errorf(
					grpccodes.FailedPrecondition,
					"cannot update template '%s': child template '%s' references parameter '%s' "+
						"which would no longer exist",
					template.GetId(), child.GetId(), childParam.GetName(),
				)
			}
			if parameter.GetSealed() {
				return grpcstatus.Errorf(
					grpccodes.FailedPrecondition,
					"cannot update template '%s': child template '%s' overrides parameter '%s' "+
						"which would become sealed",
					template.GetId(), child.GetId(), childParam.GetName(),
				)
			}
		}
	}

	return nil
}

// validateStructure checks that the template's parent reference and parameters are consistent with the inheritance
// chain. It verifies that the parent exists and is visible, that all parameters reference valid parent parameters,
// and that sealed parameters are not being overridden. This is the common validation logic shared by
// ValidateCreation and ValidateUpdate.
func (h *TemplateHelper[T, P]) validateStructure(ctx context.Context, template T) error {
	// Check that the parent template exists and is visible:
	if !template.HasParent() {
		return nil
	}
	parentRef := template.GetParent()
	if parentRef == "" {
		return grpcstatus.Errorf(grpccodes.InvalidArgument, "parent must not be empty")
	}
	parentTemplate, err := h.lookupTemplate(ctx, parentRef)
	if err != nil {
		return err
	}

	// Calculate the flattened parameters of the parent template:
	parentParameterList, err := h.calculateFlattenedParameters(ctx, parentTemplate)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Failed to calculate flattened parent parameters",
			slog.String("parent", parentRef),
			slog.Any("error", err),
		)
		return grpcstatus.Errorf(
			grpccodes.Internal,
			"failed to calculate flattened parameters for template '%s'",
			parentRef,
		)
	}

	// Create an index of parent parameters by name, so that lookups will be falster in the rest of the method:
	parentParameterIndex := make(map[string]P, len(parentParameterList))
	for _, parentParameter := range parentParameterList {
		parentParameterIndex[parentParameter.GetName()] = parentParameter
	}

	// Verify that every child parameter references a valid flattened parameter of the parent:
	for _, childParameter := range template.GetParameters() {
		parentParameter, exists := parentParameterIndex[childParameter.GetName()]
		if !exists {
			return grpcstatus.Errorf(
				grpccodes.InvalidArgument,
				"parameter '%s' does not exist in parent template '%s'",
				childParameter.GetName(), parentTemplate.GetId(),
			)
		}
		if parentParameter.GetSealed() {
			return grpcstatus.Errorf(
				grpccodes.InvalidArgument,
				"parameter '%s' is sealed in parent template '%s' and cannot be overridden",
				childParameter.GetName(), parentRef,
			)
		}
	}

	return nil
}

// ValidateDeletion checks that the template can be safely deleted. It rejects deletion if other templates reference
// it as their parent.
func (h *TemplateHelper[T, P]) ValidateDeletion(ctx context.Context, id string) error {
	response, err := h.dao.List().
		SetFilter(fmt.Sprintf("this.parent == %q", id)).
		SetLimit(1).
		Do(ctx)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Failed to check for child templates",
			slog.String("template", id),
			slog.Any("error", err),
		)
		return grpcstatus.Errorf(grpccodes.Internal, "failed to check for child templates")
	}
	if response.GetSize() > 0 {
		child := response.GetItems()[0]
		return grpcstatus.Errorf(
			grpccodes.FailedPrecondition,
			"template '%s' cannot be deleted because template '%s' references it as its parent",
			id, child.GetId(),
		)
	}
	return nil
}

// Flatten replaces the template's parameters with the fully resolved effective parameters, walking the entire
// inheritance chain. For root templates (without a parent) this is a no-op.
func (h *TemplateHelper[T, P]) Flatten(ctx context.Context, template T) error {
	if !template.HasParent() {
		return nil
	}
	flattenedParameters, err := h.calculateFlattenedParameters(ctx, template)
	if err != nil {
		return err
	}
	template.SetParameters(flattenedParameters)
	return nil
}

// CalculateAnsibleRole determines the Ansible role for a template by walking the inheritance chain from the leaf to
// the root. At each step it checks for the `osac.openshift.io/ansible-role` annotation; if found, that value is
// returned immediately. If the root is reached without finding the annotation, the root template's name is used, or
// its identifier as a last resort.
func (h *TemplateHelper[T, P]) CalculateAnsibleRole(ctx context.Context, template T) (result string, err error) {
	chain, err := h.calculateInheritanceChain(ctx, template)
	if err != nil {
		return
	}
	for _, t := range chain {
		role := t.GetMetadata().GetAnnotations()[annotations.AnsibleRole]
		if role != "" {
			result = role
			return
		}
	}
	root := chain[len(chain)-1]
	name := root.GetMetadata().GetName()
	if name != "" {
		result = name
		return
	}
	h.logger.WarnContext(
		ctx,
		"No Ansible role found for template, will use the root template's identifier",
		slog.String("template", root.GetId()),
	)
	result = root.GetId()
	return
}

// calculateFlattenedParameters walks the inheritance chain from the root to the given template and returns the merged
// (flattened) parameter definitions.
func (h *TemplateHelper[T, P]) calculateFlattenedParameters(ctx context.Context, template T) (result []P, err error) {
	chain, err := h.calculateInheritanceChain(ctx, template)
	if err != nil {
		return
	}
	index := make(map[string]P)
	for i := len(chain) - 1; i >= 0; i-- {
		for _, parameter := range chain[i].GetParameters() {
			existing, exists := index[parameter.GetName()]
			if !exists {
				index[parameter.GetName()] = proto.Clone(parameter).(P)
			} else {
				if title := parameter.GetTitle(); title != "" {
					existing.SetTitle(title)
				}
				if description := parameter.GetDescription(); description != "" {
					existing.SetDescription(description)
				}
				if parameter.GetDefault() != nil {
					existing.SetDefault(proto.Clone(parameter.GetDefault()).(*anypb.Any))
				}
				if parameter.GetSealed() {
					existing.SetSealed(true)
				}
			}
		}
	}
	list := make([]P, 0, len(index))
	for _, parameter := range index {
		list = append(list, parameter)
	}
	result = list
	return
}

// calculateInheritanceChain walks from the given template up to the root, returning the chain with the leaf first and
// root last.
func (h *TemplateHelper[T, P]) calculateInheritanceChain(ctx context.Context, template T) (result []T, err error) {
	chain := []T{template}
	visited := map[string]bool{
		template.GetId(): true,
	}
	current := template
	for current.HasParent() {
		var parent T
		parent, err = h.lookupTemplate(ctx, current.GetParent())
		if err != nil {
			return
		}
		if visited[parent.GetId()] {
			err = fmt.Errorf("cycle detected in template inheritance chain at '%s'", parent.GetId())
			return
		}
		visited[parent.GetId()] = true
		chain = append(chain, parent)
		current = parent
	}
	result = chain
	return
}

// lookupTemplate searches for a template by reference (identifier or name). It returns an error if there is no
// match or if there are multiple matches.
func (h *TemplateHelper[T, P]) lookupTemplate(ctx context.Context, ref string) (result T, err error) {
	response, err := h.dao.List().
		SetFilter(fmt.Sprintf("this.id == %[1]q || this.metadata.name == %[1]q", ref)).
		SetLimit(2).
		Do(ctx)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Failed to find template",
			slog.String("reference", ref),
			slog.Any("error", err),
		)
		err = grpcstatus.Errorf(grpccodes.Internal, "failed to find template '%s'", ref)
		return
	}
	switch response.GetSize() {
	case 0:
		err = grpcstatus.Errorf(
			grpccodes.NotFound,
			"there is no template with identifier or name '%s'",
			ref,
		)
	case 1:
		result = response.GetItems()[0]
	default:
		err = grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"there are multiple templates with identifier or name '%s'",
			ref,
		)
	}
	return
}

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
	"net"

	"github.com/prometheus/client_golang/prometheus"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/auth"
	"github.com/osac-project/fulfillment-service/internal/database/dao"
	"github.com/osac-project/fulfillment-service/internal/events"
)

type PrivateTenantsServerBuilder struct {
	logger            *slog.Logger
	notifier          events.Notifier
	attributionLogic  auth.AttributionLogic
	tenancyLogic      auth.TenancyLogic
	metricsRegisterer prometheus.Registerer
}

var _ privatev1.TenantsServer = (*PrivateTenantsServer)(nil)

type PrivateTenantsServer struct {
	privatev1.UnimplementedTenantsServer
	logger  *slog.Logger
	generic *GenericServer[*privatev1.Tenant]
	dao     *dao.GenericDAO[*privatev1.Tenant]
}

func NewPrivateTenantsServer() *PrivateTenantsServerBuilder {
	return &PrivateTenantsServerBuilder{}
}

func (b *PrivateTenantsServerBuilder) SetLogger(value *slog.Logger) *PrivateTenantsServerBuilder {
	b.logger = value
	return b
}

func (b *PrivateTenantsServerBuilder) SetNotifier(value events.Notifier) *PrivateTenantsServerBuilder {
	b.notifier = value
	return b
}

func (b *PrivateTenantsServerBuilder) SetAttributionLogic(value auth.AttributionLogic) *PrivateTenantsServerBuilder {
	b.attributionLogic = value
	return b
}

func (b *PrivateTenantsServerBuilder) SetTenancyLogic(value auth.TenancyLogic) *PrivateTenantsServerBuilder {
	b.tenancyLogic = value
	return b
}

// SetMetricsRegisterer sets the Prometheus registerer used to register the metrics for the underlying database
// access objects. This is optional. If not set, no metrics will be recorded.
func (b *PrivateTenantsServerBuilder) SetMetricsRegisterer(value prometheus.Registerer) *PrivateTenantsServerBuilder {
	b.metricsRegisterer = value
	return b
}

func (b *PrivateTenantsServerBuilder) Build() (result *PrivateTenantsServer, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.tenancyLogic == nil {
		err = errors.New("tenancy logic is mandatory")
		return
	}

	// Create the generic server:
	generic, err := NewGenericServer[*privatev1.Tenant]().
		SetLogger(b.logger).
		SetService(privatev1.Tenants_ServiceDesc.ServiceName).
		SetTableName("tenants").
		SetNotifier(b.notifier).
		SetAttributionLogic(b.attributionLogic).
		SetTenancyLogic(b.tenancyLogic).
		SetMetricsRegisterer(b.metricsRegisterer).
		Build()
	if err != nil {
		return
	}

	// Create the DAO:
	dao, err := dao.NewGenericDAO[*privatev1.Tenant]().
		SetLogger(b.logger).
		SetTableName("tenants").
		SetTenancyLogic(b.tenancyLogic).
		SetMetricsRegisterer(b.metricsRegisterer).
		Build()
	if err != nil {
		return
	}

	// Create and populate the object:
	result = &PrivateTenantsServer{
		logger:  b.logger,
		generic: generic,
		dao:     dao,
	}
	return
}

func (s *PrivateTenantsServer) List(ctx context.Context,
	request *privatev1.TenantsListRequest) (response *privatev1.TenantsListResponse, err error) {
	err = s.generic.List(ctx, request, &response)
	return
}

func (s *PrivateTenantsServer) Get(ctx context.Context,
	request *privatev1.TenantsGetRequest) (response *privatev1.TenantsGetResponse, err error) {
	err = s.generic.Get(ctx, request, &response)
	return
}

func (s *PrivateTenantsServer) Create(ctx context.Context,
	request *privatev1.TenantsCreateRequest) (response *privatev1.TenantsCreateResponse, err error) {
	// For tenants the name is mandatory:
	object := request.GetObject()
	metadata := object.GetMetadata()
	name := metadata.GetName()
	if name == "" {
		err = grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"field 'metadata.name' is mandatory",
		)
		return
	}

	// For tenants the identifier must be empty or equal to the name. If it is empty it will be set to the name.
	id := object.GetId()
	if id != "" && id != name {
		err = grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"field 'id' must be empty or equal to field 'metadata.name'",
		)
		return
	}
	if id == "" {
		object.SetId(name)
	}

	// The tenant of a tenant must be itself, so either empty or equal to the name. If it is empty it will be set to
	// the name.
	tenant := metadata.GetTenant()
	if tenant != "" && tenant != name {
		err = grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"field 'metadata.tenant' must be empty or equal to field 'metadata.name'",
		)
		return
	}
	if tenant == "" {
		metadata.SetTenant(name)
	}

	// Validate the domains:
	err = s.validateDomains(object.GetSpec().GetDomains())
	if err != nil {
		return
	}

	// Delegate to the generic server:
	err = s.generic.Create(ctx, request, &response)
	return
}

func (s *PrivateTenantsServer) Update(ctx context.Context,
	request *privatev1.TenantsUpdateRequest) (response *privatev1.TenantsUpdateResponse, err error) {
	// Validate the domains:
	err = s.validateDomains(request.GetObject().GetSpec().GetDomains())
	if err != nil {
		return
	}

	// Delegate to the generic server:
	err = s.generic.Update(ctx, request, &response)
	return
}

func (s *PrivateTenantsServer) Delete(ctx context.Context,
	request *privatev1.TenantsDeleteRequest) (response *privatev1.TenantsDeleteResponse, err error) {
	err = s.generic.Delete(ctx, request, &response)
	return
}

func (s *PrivateTenantsServer) Signal(ctx context.Context,
	request *privatev1.TenantsSignalRequest) (response *privatev1.TenantsSignalResponse, err error) {
	err = s.generic.Signal(ctx, request, &response)
	return
}

// validateDomains checks that all domains in the list are valid DNS hostnames and that there are no duplicates.
// It is safe to call with a nil or empty slice.
func (s *PrivateTenantsServer) validateDomains(domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(domains))
	for i, domain := range domains {
		if err := s.validateDomain(domain, i); err != nil {
			return err
		}
		if seen[domain] {
			return grpcstatus.Errorf(
				grpccodes.InvalidArgument,
				"field 'spec.domains' contains duplicate domain '%s'",
				domain,
			)
		}
		seen[domain] = true
	}
	return nil
}

// validateDomain checks that a single domain is a syntactically valid DNS hostname suitable for use as an
// e-mail domain.
func (s *PrivateTenantsServer) validateDomain(domain string, index int) error {
	if domain == "" {
		return grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"field 'spec.domains[%d]' must not be empty",
			index,
		)
	}
	if len(domain) > 253 {
		return grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"field 'spec.domains[%d]' must be at most 253 characters long, but '%s' has %d characters",
			index, domain, len(domain),
		)
	}
	if net.ParseIP(domain) != nil {
		return grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"field 'spec.domains[%d]' must be a DNS hostname, not an IP address: '%s'",
			index, domain,
		)
	}

	labels := s.splitDomainLabels(domain)
	if len(labels) < 2 {
		return grpcstatus.Errorf(
			grpccodes.InvalidArgument,
			"field 'spec.domains[%d]' must have at least two labels (e.g. 'example.com'), but got '%s'",
			index, domain,
		)
	}
	for _, label := range labels {
		if err := s.validateDomainLabel(label); err != nil {
			return grpcstatus.Errorf(
				grpccodes.InvalidArgument,
				"field 'spec.domains[%d]' contains invalid label in '%s': %s",
				index, domain, err,
			)
		}
	}
	return nil
}

// splitDomainLabels splits a domain name into its dot-separated labels.
func (s *PrivateTenantsServer) splitDomainLabels(domain string) []string {
	var labels []string
	start := 0
	for i := 0; i <= len(domain); i++ {
		if i == len(domain) || domain[i] == '.' {
			labels = append(labels, domain[start:i])
			start = i + 1
		}
	}
	return labels
}

// validateDomainLabel checks that a single DNS label is valid per RFC 1035.
func (s *PrivateTenantsServer) validateDomainLabel(label string) error {
	if len(label) == 0 {
		return fmt.Errorf("label must not be empty")
	}
	if len(label) > 63 {
		return fmt.Errorf("label must be at most 63 characters long, but has %d", len(label))
	}
	for i, c := range label {
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		isHyphen := c == '-'
		if !isLower && !isDigit && !isHyphen {
			return fmt.Errorf(
				"label must only contain lowercase letters (a-z), digits (0-9) and hyphens (-), "+
					"but contains '%c' at position %d",
				c, i,
			)
		}
	}
	if label[0] == '-' {
		return fmt.Errorf("label must not start with a hyphen")
	}
	if label[len(label)-1] == '-' {
		return fmt.Errorf("label must not end with a hyphen")
	}
	return nil
}

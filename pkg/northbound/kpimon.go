// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: LicenseRef-ONF-Member-1.0

package northbound

import (
	"context"
	"fmt"
	kpimonapi "github.com/onosproject/onos-api/go/onos/kpimon"
	"github.com/onosproject/onos-kpimon/pkg/controller"
	service "github.com/onosproject/onos-lib-go/pkg/northbound"
	"google.golang.org/grpc"
)

// NewService returns a new KPIMON interface service.
func NewService(ctrl *controller.KpiMonCtrl) service.Service {
	return &Service{
		Ctrl: ctrl,
	}
}

// Service is a service implementation for administration.
type Service struct {
	service.Service
	Ctrl *controller.KpiMonCtrl
}

// Register registers the Service with the gRPC server.
func (s Service) Register(r *grpc.Server) {
	server := &Server{
		Ctrl: s.Ctrl,
	}
	kpimonapi.RegisterKpimonServer(r, server)
}

// Server implements the KPIMON gRPC service for administrative facilities.
type Server struct {
	Ctrl *controller.KpiMonCtrl
}

// GetNumActiveUEs returns how many UEs are acctive
func (s Server) GetNumActiveUEs(ctx context.Context, request *kpimonapi.GetRequest) (*kpimonapi.GetResponse, error) {

	numActiveUEs := make(map[string]uint64)

	s.Ctrl.KpiMonMutex.RLock()
	for k, v := range s.Ctrl.KpiMonResults {
		id := fmt.Sprintf("%s", k)
		numActiveUEs[id] = uint64(v)
	}
	s.Ctrl.KpiMonMutex.RUnlock()

	response := &kpimonapi.GetResponse{
		Object: &kpimonapi.Object{
			Id:         "kpimon",
			Revision:   0,
			Attributes: numActiveUEs,
		},
	}

	return response, nil
}

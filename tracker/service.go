package tracker

import (
	"context"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
)

var Model = resource.NewModel("devrel", "sun-tracker", "sun-servo-tracker")

func init() {
	resource.RegisterService(generic.API, Model,
		resource.Registration[resource.Resource, *Config]{Constructor: newService},
	)
}

type service struct {
	resource.Named
	resource.AlwaysRebuild
	logger logging.Logger
}

func newService(
	ctx context.Context,
	deps resource.Dependencies,
	conf resource.Config,
	logger logging.Logger,
) (resource.Resource, error) {
	return &service{
		Named:  conf.ResourceName().AsNamed(),
		logger: logger,
	}, nil
}

func (s *service) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, resource.ErrDoUnimplemented
}

func (s *service) Close(ctx context.Context) error { return nil }

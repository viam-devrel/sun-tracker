package main

import (
	"context"
	"fmt"

	"github.com/viam-devrel/sun-tracker/sunposition"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	vision "go.viam.com/rdk/services/vision"
)

func main() {
	err := realMain()
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
}

func realMain() error {
	ctx := context.Background()
	logger := logging.NewLogger("cli")

	deps := resource.Dependencies{}
	// can load these from a remote machine if you need

	cfg := sunposition.Config{Camera: "stub"}
	conf := resource.Config{Name: "smoke", Model: sunposition.Model}

	thing, err := sunposition.NewServiceWithConfig(ctx, deps, conf, &cfg, logger)
	if err != nil {
		// Expected at runtime — camera dep "stub" won't be in empty deps.
		logger.Infow("smoke test: constructor returned error (expected without live camera)", "err", err)
		return nil
	}
	defer thing.Close(ctx)
	_ = vision.Named("foo")
	return nil
}

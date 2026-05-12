package main

import (
	"github.com/viam-devrel/sun-tracker/sunposition"
	"github.com/viam-devrel/sun-tracker/tracker"

	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/vision"
)

func main() {
	module.ModularMain(
		resource.APIModel{API: vision.API, Model: sunposition.Model},
		resource.APIModel{API: generic.API, Model: tracker.Model},
	)
}

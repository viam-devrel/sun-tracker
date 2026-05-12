package main

import (
	"github.com/viam-devrel/sun-tracker"

	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	vision "go.viam.com/rdk/services/vision"
)

func main() {
	// ModularMain can take multiple APIModel arguments, if your module implements multiple models.
	module.ModularMain(resource.APIModel{API: vision.API, Model: suntracker.SunPosition})
}

package main

import (
	"github.com/viam-devrel/sun-tracker/sunposition"

	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	vision "go.viam.com/rdk/services/vision"
)

func main() {
	module.ModularMain(resource.APIModel{API: vision.API, Model: sunposition.Model})
}

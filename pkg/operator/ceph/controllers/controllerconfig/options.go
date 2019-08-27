package controllerconfig

import (
	"github.com/rook/rook/pkg/clusterd"
)

// Options passed to the controller when associating it with the manager.
type Options struct {
	Context           *clusterd.Context
	OperatorNamespace string
}

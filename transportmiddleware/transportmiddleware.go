package transportmiddleware

import (
	"context"

	"github.com/yarpc/yab/transport"
)

var registeredMiddleware TransportMiddleware

// Register sets the provided transport middleware to be used on future
// calls to Apply(). Calls to Register() will overwrite previously registered
// middlewares; that is, only one middleware is allowed at a time.
func Register(tm TransportMiddleware) {
	registeredMiddleware = tm
}

// TransportMiddleware
type TransportMiddleware interface {
	// Apply mutates and returns the passed Request object.
	//
	// Implementations are prevented from modifying the Request object
	// in the case that an error is encountered.
	Apply(ctx context.Context, req *transport.Request) (*transport.Request, error)
}

// Apply mutates a Request using the previously registered TransportMiddleware.
func Apply(ctx context.Context, req *transport.Request) (*transport.Request, error) {
	if registeredMiddleware == nil {
		return req, nil
	}
	return registeredMiddleware.Apply(ctx, req)
}

// Package transportmiddleware provides an interface for hooking into
// outgoing yab requests.
package transportmiddleware

import (
	"context"
	"sync"

	"github.com/yarpc/yab/transport"
)

// stores the currently registered middleware
var registeredMiddleware Interface

// serializes access to the currently registered middleware
var registerLock sync.RWMutex

// Register sets the provided transport middleware to be used on future
// calls to Apply(). Calls to Register() will overwrite previously registered
// middlewares; that is, only one middleware is allowed at a time.
func Register(newMW Interface) (restore func()) {
	registerLock.Lock()
	oldMW := registeredMiddleware
	registeredMiddleware = newMW
	registerLock.Unlock()

	return func() {
		registerLock.Lock()
		registeredMiddleware = oldMW
		registerLock.Unlock()
	}
}

// Interface allows for its implementors to modify an in-flight Request.
type Interface interface {
	// Apply mutates and returns the passed Request object.
	Apply(ctx context.Context, req *transport.Request) (*transport.Request, error)
}

// Apply mutates a Request using the previously registered Interface.
func Apply(ctx context.Context, req *transport.Request) (*transport.Request, error) {
	registerLock.RLock()
	mw := registeredMiddleware
	registerLock.RUnlock()

	if mw == nil {
		return req, nil
	}
	return mw.Apply(ctx, req)
}

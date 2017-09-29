package transport

import "context"

// stores the currently registered middleware
var registeredInterceptor RequestInterceptor

// RegisterInterceptor sets the provided request interceptor to be used on future
// calls to Apply(). Calls to Register() will overwrite previously registered
// middlewares; that is, only one middleware is allowed at a time.
// Returns a function to undo the change made by this call.
func RegisterInterceptor(newRI RequestInterceptor) (restore func()) {
	oldRI := registeredInterceptor
	registeredInterceptor = newRI
	return func() {
		registeredInterceptor = oldRI
	}
}

// RequestInterceptor allows for its implementors to modify a pre-flight Request.
type RequestInterceptor interface {
	// Apply mutates and returns the passed Request object.
	Apply(ctx context.Context, req *Request) (*Request, error)
}

// ApplyMiddleware mutates a Request using the previously registered RequestInterceptor.
func ApplyMiddleware(ctx context.Context, req *Request) (*Request, error) {
	if registeredInterceptor == nil {
		return req, nil
	}
	return registeredInterceptor.Apply(ctx, req)
}

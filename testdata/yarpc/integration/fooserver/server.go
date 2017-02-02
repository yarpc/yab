// Code generated by thriftrw-plugin-yarpc
// @generated

package fooserver

import (
	"context"
	"go.uber.org/thriftrw/wire"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/encoding/thrift"
	"github.com/yarpc/yab/testdata/yarpc/integration"
)

// Interface is the server-side interface for the Foo service.
type Interface interface {
	Bar(
		ctx context.Context,
		Arg *int32,
	) (int32, error)
}

// New prepares an implementation of the Foo service for
// registration.
//
// 	handler := FooHandler{}
// 	dispatcher.Register(fooserver.New(handler))
func New(impl Interface, opts ...thrift.RegisterOption) []transport.Procedure {
	h := handler{impl}
	service := thrift.Service{
		Name: "Foo",
		Methods: []thrift.Method{

			thrift.Method{
				Name: "bar",
				HandlerSpec: thrift.HandlerSpec{

					Type:  transport.Unary,
					Unary: thrift.UnaryHandler(h.Bar),
				},
				Signature: "Bar(Arg *int32) (int32)",
			},
		},
	}

	procedures := make([]transport.Procedure, 0, 1)
	procedures = append(procedures, thrift.BuildProcedures(service, opts...)...)
	return procedures
}

type handler struct{ impl Interface }

func (h handler) Bar(ctx context.Context, body wire.Value) (thrift.Response, error) {
	var args integration.Foo_Bar_Args
	if err := args.FromWire(body); err != nil {
		return thrift.Response{}, err
	}

	success, err := h.impl.Bar(ctx, args.Arg)

	hadError := err != nil
	result, err := integration.Foo_Bar_Helper.WrapResponse(success, err)

	var response thrift.Response
	if err == nil {
		response.IsApplicationError = hadError
		response.Body = result
	}
	return response, err
}
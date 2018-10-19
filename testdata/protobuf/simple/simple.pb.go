// Code generated by protoc-gen-go. DO NOT EDIT.
// source: testdata/protobuf/simple/simple.proto

package simple

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type Foo struct {
	Test                 int32    `protobuf:"varint,1,opt,name=test,proto3" json:"test,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Foo) Reset()         { *m = Foo{} }
func (m *Foo) String() string { return proto.CompactTextString(m) }
func (*Foo) ProtoMessage()    {}
func (*Foo) Descriptor() ([]byte, []int) {
	return fileDescriptor_simple_fa8f59242fada772, []int{0}
}
func (m *Foo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Foo.Unmarshal(m, b)
}
func (m *Foo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Foo.Marshal(b, m, deterministic)
}
func (dst *Foo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Foo.Merge(dst, src)
}
func (m *Foo) XXX_Size() int {
	return xxx_messageInfo_Foo.Size(m)
}
func (m *Foo) XXX_DiscardUnknown() {
	xxx_messageInfo_Foo.DiscardUnknown(m)
}

var xxx_messageInfo_Foo proto.InternalMessageInfo

func (m *Foo) GetTest() int32 {
	if m != nil {
		return m.Test
	}
	return 0
}

func init() {
	proto.RegisterType((*Foo)(nil), "Foo")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// BarClient is the client API for Bar service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type BarClient interface {
	Baz(ctx context.Context, in *Foo, opts ...grpc.CallOption) (*Foo, error)
}

type barClient struct {
	cc *grpc.ClientConn
}

func NewBarClient(cc *grpc.ClientConn) BarClient {
	return &barClient{cc}
}

func (c *barClient) Baz(ctx context.Context, in *Foo, opts ...grpc.CallOption) (*Foo, error) {
	out := new(Foo)
	err := c.cc.Invoke(ctx, "/Bar/Baz", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BarServer is the server API for Bar service.
type BarServer interface {
	Baz(context.Context, *Foo) (*Foo, error)
}

func RegisterBarServer(s *grpc.Server, srv BarServer) {
	s.RegisterService(&_Bar_serviceDesc, srv)
}

func _Bar_Baz_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Foo)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BarServer).Baz(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/Bar/Baz",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BarServer).Baz(ctx, req.(*Foo))
	}
	return interceptor(ctx, in, info, handler)
}

var _Bar_serviceDesc = grpc.ServiceDesc{
	ServiceName: "Bar",
	HandlerType: (*BarServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Baz",
			Handler:    _Bar_Baz_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "testdata/protobuf/simple/simple.proto",
}

func init() {
	proto.RegisterFile("testdata/protobuf/simple/simple.proto", fileDescriptor_simple_fa8f59242fada772)
}

var fileDescriptor_simple_fa8f59242fada772 = []byte{
	// 102 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x52, 0x2d, 0x49, 0x2d, 0x2e,
	0x49, 0x49, 0x2c, 0x49, 0xd4, 0x2f, 0x28, 0xca, 0x2f, 0xc9, 0x4f, 0x2a, 0x4d, 0xd3, 0x2f, 0xce,
	0xcc, 0x2d, 0xc8, 0x49, 0x85, 0x52, 0x7a, 0x60, 0x61, 0x25, 0x49, 0x2e, 0x66, 0xb7, 0xfc, 0x7c,
	0x21, 0x21, 0x2e, 0x16, 0x90, 0x7a, 0x09, 0x46, 0x05, 0x46, 0x0d, 0xd6, 0x20, 0x30, 0xdb, 0x48,
	0x82, 0x8b, 0xd9, 0x29, 0xb1, 0x48, 0x48, 0x10, 0x44, 0x55, 0x09, 0xb1, 0xe8, 0xb9, 0xe5, 0xe7,
	0x4b, 0x81, 0xc9, 0x24, 0x36, 0xb0, 0x5e, 0x63, 0x40, 0x00, 0x00, 0x00, 0xff, 0xff, 0x7f, 0x65,
	0xb1, 0xc0, 0x64, 0x00, 0x00, 0x00,
}

package protobuf

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func TestReflection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	require.NoError(t, err)
	s := grpc.NewServer()
	reflection.Register(s)
	go s.Serve(ln)

	source, err := NewDescriptorProviderReflection(ReflectionArgs{
		Timeout: time.Second,
		Peers:   []string{ln.Addr().String()},
	})
	assert.NoError(t, err)
	assert.NotNil(t, source)

	result, err := source.FindSymbol("grpc.reflection.v1alpha.ServerReflectionRequest")
	assert.NoError(t, err)
	assert.NotNil(t, result)

	result, err = source.FindSymbol("wat")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Symbol not found: wat")
	assert.Nil(t, result)
}

func TestReflectionMultiplePeers(t *testing.T) {
	listenRefuser, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to listen on a port")
	noListen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to listen on a port")
	noListen.Close()

	got, err := NewDescriptorProviderReflection(ReflectionArgs{
		Timeout: time.Second,
		Peers:   []string{noListen.Addr().String(), listenRefuser.Addr().String()},
	})

	require.NoError(t, err)
	require.NotNil(t, got)

	go func() {
		conn, _ := listenRefuser.Accept()
		conn.Close()
	}()
	_, err = got.FindSymbol("some-symbol")
	require.Error(t, err)
}

func TestReflectionClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to listen on a port")
	ln.Close()

	got, err := NewDescriptorProviderReflection(ReflectionArgs{
		Timeout: time.Second,
		Peers:   []string{ln.Addr().String()},
	})

	assert.Error(t, err)
	assert.Nil(t, got)
}

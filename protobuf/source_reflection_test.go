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
	require.NoError(t, err)
	defer ln.Close()

	s := grpc.NewServer()
	reflection.Register(s)
	go s.Serve(ln)

	// Ensure that all streams are closed by the end of the test.
	defer s.GracefulStop()

	source, err := NewDescriptorProviderReflection(ReflectionArgs{
		Timeout: time.Second,
		Peers:   []string{ln.Addr().String()},
	})
	require.NoError(t, err)
	require.NotNil(t, source)

	// Close the streaming reflect call to ensure GracefulStop doesn't block.
	defer source.Close()

	t.Run("valid service", func(t *testing.T) {
		result, err := source.FindService("grpc.reflection.v1alpha.ServerReflection")
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("non-service symbol", func(t *testing.T) {
		result, err := source.FindService("grpc.reflection.v1alpha.ServerReflectionRequest")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), `could not find gRPC service`)
	})

	t.Run("no such symbol", func(t *testing.T) {
		result, err := source.FindService("wat")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), `could not find gRPC service "wat"`)
	})
}

func TestReflectionWithProtocolInPeer(t *testing.T) {
	source, err := NewDescriptorProviderReflection(ReflectionArgs{
		Timeout: time.Second,
		Peers:   []string{"grpc://127.0.0.1:12345"},
	})
	assert.Nil(t, source)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "peer contains scheme")
}

func TestReflectionClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to listen on a port")
	ln.Close()

	got, err := NewDescriptorProviderReflection(ReflectionArgs{
		Timeout: time.Second,
		Peers:   []string{ln.Addr().String()},
	})

	assert.Contains(t, err.Error(), "could not reach reflection server")
	assert.Nil(t, got)
}

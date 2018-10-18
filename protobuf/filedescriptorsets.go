package protobuf

import (
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
)

// NewDescriptorProviderFileDescriptorSetBins creates a DescriptorSource that is backed by the named files, whose contents
// are encoded FileDescriptorSet protos.
func NewDescriptorProviderFileDescriptorSetBins(fileNames ...string) (DescriptorProvider, error) {
	files := &descriptor.FileDescriptorSet{}
	for _, fileName := range fileNames {
		b, err := ioutil.ReadFile(fileName)
		if err != nil {
			return nil, fmt.Errorf("could not load protoset file %q: %v", fileName, err)
		}
		var fs descriptor.FileDescriptorSet
		err = proto.Unmarshal(b, &fs)
		if err != nil {
			return nil, fmt.Errorf("could not parse contents of protoset file %q: %v", fileName, err)
		}
		files.File = append(files.File, fs.File...)
	}
	return NewDescriptorProviderFileDescriptorSet(files)
}

// NewDescriptorProviderFileDescriptorSet creates a DescriptorSource that is backed by the FileDescriptorSet.
func NewDescriptorProviderFileDescriptorSet(files *descriptor.FileDescriptorSet) (DescriptorProvider, error) {
	unresolved := make(map[string]*descriptor.FileDescriptorProto, len(files.File))
	for _, fd := range files.File {
		unresolved[fd.GetName()] = fd
	}
	resolved := map[string]*desc.FileDescriptor{}
	for _, fd := range files.File {
		if _, err := resolveFileDescriptor(unresolved, resolved, fd.GetName()); err != nil {
			return nil, err
		}
	}
	return &fileSource{files: resolved}, nil
}

func resolveFileDescriptor(unresolved map[string]*descriptor.FileDescriptorProto, resolved map[string]*desc.FileDescriptor, filename string) (*desc.FileDescriptor, error) {
	if r, ok := resolved[filename]; ok {
		return r, nil
	}
	fd, ok := unresolved[filename]
	if !ok {
		return nil, fmt.Errorf("no descriptor found for %q", filename)
	}
	deps := make([]*desc.FileDescriptor, 0, len(fd.GetDependency()))
	for _, dep := range fd.GetDependency() {
		depFd, err := resolveFileDescriptor(unresolved, resolved, dep)
		if err != nil {
			return nil, err
		}
		deps = append(deps, depFd)
	}
	result, err := desc.CreateFileDescriptor(fd, deps...)
	if err != nil {
		return nil, err
	}
	resolved[filename] = result
	return result, nil
}

type fileSource struct {
	files map[string]*desc.FileDescriptor
}

func (fs *fileSource) FindSymbol(fullyQualifiedName string) (desc.Descriptor, error) {
	for _, fd := range fs.files {
		if dsc := fd.FindSymbol(fullyQualifiedName); dsc != nil {
			return dsc, nil
		}
	}
	return nil, fmt.Errorf("symbol not found: %q", fullyQualifiedName)
}

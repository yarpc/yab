package protobuf

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// ProtoDescriptorSourceFromProtoFiles runs protoc and creates a filedescriptor
func ProtoDescriptorSourceFromProtoFiles(importPaths []string, fileNames ...string) (ProtoDescriptorSource, error) {
	protocPath, err := exec.LookPath("protoc")
	if err != nil {
		return nil, fmt.Errorf("can't find protoc: %s", err)
	}

	tempdir, err := ioutil.TempDir("", "yab")
	if err != nil {
		return nil, fmt.Errorf("could not create temporary directory to compile proto: %s", err)
	}
	defer os.RemoveAll(tempdir)

	fdFile := filepath.Join(tempdir, "proto.bin")
	argv := []string{
		fmt.Sprintf("--descriptor_set_out=%s", fdFile),
		"--include_imports",
		"--include_source_info",
	}
	for _, importPath := range importPaths {
		argv = append(argv, fmt.Sprintf("--proto_path=%s", importPath))
	}
	argv = append(argv, fileNames...)

	cmd := exec.Command(protocPath, argv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("could not parse provided files. protoc output: %s", out)
	}

	b, err := ioutil.ReadFile(fdFile)
	if err != nil {
		return nil, fmt.Errorf("could not load protoset file %q: %v", fdFile, err)
	}
	var fs descriptor.FileDescriptorSet
	err = proto.Unmarshal(b, &fs)
	if err != nil {
		return nil, fmt.Errorf("could not parse contents of protoset file %q: %v", fdFile, err)
	}

	return ProtoDescriptorSourceFromFileDescriptorSet(&descriptor.FileDescriptorSet{
		File: fs.File,
	})
}

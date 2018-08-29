#!/usr/bin/env bash
#
# generate compiled binary protodescriptors, protoc required then commit files for test usage
# 
set -euo pipefail

THIS_DIR="$(cd -P -- "$(dirname -- "$0")" && pwd -P)"

# simple
(cd "$THIS_DIR/simple" && protoc -osimple.proto.bin simple.proto)

# as expected
(cd "$THIS_DIR/dependencies" && protoc --include_imports --descriptor_set_out=combined.bin main.proto)

# no --include_imports
(cd "$THIS_DIR/dependencies" && protoc --descriptor_set_out=main.proto.bin main.proto)

# just the dependency to check using multiple precompiled files
(cd "$THIS_DIR/dependencies" && protoc --descriptor_set_out=dep.proto.bin dep.proto)

# search and replace message name Foo with fox, to simulate errors where proto files were named similarly but
# have different contents
(cd "$THIS_DIR/dependencies" && perl -p -e  's/Foo/Fox/g' dep.proto.bin > other.bin)

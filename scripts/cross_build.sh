#!/bin/bash

mkdir -p build
for GOOS in linux darwin
do
  for GOARCH in amd64
    do
    echo Building $GOOS/$GOARCH
    GOOS=$GOOS GOARCH=$GOARCH go build -o build/yab-$GOOS-$GOARCH .
    zip build/yab-$GOOS-$GOARCH.zip build/yab-$GOOS-$GOARCH
  done
done


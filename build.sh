#!/bin/bash

GO_VERSION=$(go env GOVERSION | cut -b "3-")
GO_MAJOR_VERSION=$(cut -d '.' -f 1,2 <<< "$GO_VERSION")
TAG=$(git tag | sort -V | tail -1)

echo
echo Go version is $GO_VERSION, major version is $GO_MAJOR_VERSION
echo Tag is $TAG

echo
echo Building ancientlore/chatty:$TAG
docker buildx build --build-arg GO_VERSION=$GO_VERSION --build-arg IMG_VERSION=$GO_MAJOR_VERSION --platform linux/amd64,linux/arm64 -t ancientlore/chatty:$TAG . || exit 1

gum confirm "Push?" || exit 1

echo
echo Pushing ancientlore/chatty:$TAG
docker push ancientlore/chatty:$TAG || exit 1

echo
echo Tagging ancientlore/chatty:latest
docker tag ancientlore/chatty:$TAG ancientlore/chatty:latest || exit 1

echo
echo Pushing ancientlore/chatty:latest
docker push ancientlore/chatty:latest || exit 1

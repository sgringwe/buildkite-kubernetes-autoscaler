#!/bin/sh

set -ex

USERNAME=sgringwe
IMAGE=buildkite-kubernetes-autoscaler

version=`cat VERSION`
echo "version: $version"

# Build the golang binary first
./build.sh

# run build
docker build -t $USERNAME/$IMAGE:latest .

# tag it
# git add -A
# git commit -m "version $version"
# git tag -a "$version" -m "version $version"
# git push
# git push --tags

docker tag $USERNAME/$IMAGE:latest $USERNAME/$IMAGE:$version

# push it
docker push $USERNAME/$IMAGE:latest
docker push $USERNAME/$IMAGE:$version
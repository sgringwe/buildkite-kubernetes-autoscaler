#!/bin/bash

# Expects the directory structure:
# .
# └── projectname
#     ├── build.sh
#     └── src
#         ├── custompackage
#         │   └── custompackage.go
#         └── main
#             └── main.go
#
# This will build a binary called "projectname" at "projectname/bin/projectname".
#
# The final structure will look like:
#
# .
# └── projectname
#     ├── bin
#     │   └── projectname
#     ├── build.sh
#     └── src
#         ├── custompackage
#         │   └── custompackage.go
#         └── main
#             └── main.go
#

# Save the pwd before we run anything
PRE_PWD=`pwd`

# Determine the build script's actual directory, following symlinks
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
BUILD_DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"

# Derive the project name from the directory
PROJECT="$(basename $BUILD_DIR)"

# Setup the environment for the build
GOPATH=$BUILD_DIR:$GOPATH

# Build the project
cd $BUILD_DIR
mkdir -p bin

# Cross-platform build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/$PROJECT ./cmd/buildkite-kubernetes-autoscaler

EXIT_STATUS=$?

if [ $EXIT_STATUS == 0 ]; then
  echo "Build succeeded"
else
  echo "Build failed"
fi

# Change back to where we were
cd $PRE_PWD

exit $EXIT_STATUS
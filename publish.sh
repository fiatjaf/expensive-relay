#!/bin/bash
set -e

# Only allow release if all code is checked in
NEEDS_COMMIT=$( git diff-index --quiet HEAD -- && echo 0 || echo 1 )
if [[ $NEEDS_COMMIT -ne 0 ]]; then
  echo "Cannot publish with uncommitted changes."
  exit 1
fi
# TODO: ensure release branch

# Set version number
DATE="$( git log -1 --format="%at" )"
DATE="$( python -c "import time; print(time.strftime('%-y.%-j', time.localtime($DATE)))" )"
HASH="$( git rev-parse --short HEAD )"
COMMIT_NUM=$( git rev-list --count HEAD )
VER=$DATE"."$COMMIT_NUM

docker build --platform linux/amd64 -t rdbell/expensive-relay:latest -t rdbell/expensive-relay:$VER . &&
docker push rdbell/expensive-relay:$VER
docker push rdbell/expensive-relay:latest

exit 0

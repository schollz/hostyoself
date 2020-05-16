#!/bin/bash
#
# Wrapper to build our contianer
#

# Errors are fatal
set -e

pushd $(dirname $0)/.. > /dev/null

docker build . -t hostyoself



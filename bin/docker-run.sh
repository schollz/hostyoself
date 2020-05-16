#!/bin/bash
#
# Wrapper to build our contianer
#

# Errors are fatal
set -e

pushd $(dirname $0)/.. > /dev/null

./bin/docker-build.sh

#
# We're setting the current directory to be /data for testing/development.
#
echo "# "
echo "# Mounting current directory as /data for testing..."
echo "# "
echo "# Serve with up $0 host --folder /data"
echo "# "
docker run -v $(pwd):/data hostyoself $@



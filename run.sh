#!/bin/bash
set -e

APPENV=${APPENV:-mehtrics}

/opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$APPENV > /$APPENV

source /$APPENV && \
	/mehtrics

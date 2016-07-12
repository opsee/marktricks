#!/bin/bash
set -e

APPENV=${APPENV:-mehtricsenv}

/opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$APPENV > /$APPENV

source /$APPENV && \
    /opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$MEHTRICS_CERT > /$MEHTRICS_CERT && \
    /opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$MEHTRICS_CERT_KEY > /$MEHTRICS_CERT_KEY && \
    chmod 600 /$MEHTRICS_CERT_KEY && \
	/worker

#!/bin/bash
set -e

APPENV=${APPENV:-marktricksenv}

/opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$APPENV > /$APPENV

source /$APPENV && \
    /opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$MARKTRICKS_CERT > /$MARKTRICKS_CERT && \
    /opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$MARKTRICKS_CERT_KEY > /$MARKTRICKS_CERT_KEY && \
    chmod 600 /$MARKTRICKS_CERT_KEY && \
	/worker

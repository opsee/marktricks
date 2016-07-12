FROM alpine:3.3

RUN apk add --update bash ca-certificates curl
RUN mkdir -p /opt/bin && \
		curl -Lo /opt/bin/s3kms https://s3-us-west-2.amazonaws.com/opsee-releases/go/vinz-clortho/s3kms-linux-amd64 && \
    chmod 755 /opt/bin/s3kms

ENV MEHTRICS_NSQLOOKUPD_ADDRS ""
ENV MEHTRICS_NSQD_HOST ""
ENV METRICS_KAIROSDB_ADDRESS ""
ENV METRICS_ADDRESS ""
ENV MEHTRICS_CERT="cert.pem"
ENV MEHTRICS_CERT_KEY="key.pem"
ENV APPENV ""

COPY run.sh /
COPY target/linux/amd64/bin/* /
COPY *.pem /

CMD ["/worker"]

FROM scratch
COPY api /bin/api
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 1000:1000
ENTRYPOINT []

FROM scratch
COPY api worker monitor test-issuer /bin/
USER 1000:1000

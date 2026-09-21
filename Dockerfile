# Runtime image for the headless local API (`partstable serve`) — FM-2.
# Built and pushed by goreleaser during the release pipeline: the linux
# binary is copied into the docker build context by goreleaser itself,
# so a plain `docker build` outside goreleaser has nothing to copy.
FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        libgtk-4-1 \
        libwebkitgtk-6.0-4 \
        libsoup-3.0-0 \
    && rm -rf /var/lib/apt/lists/*

COPY partstable /usr/local/bin/partstable
RUN chmod +x /usr/local/bin/partstable

# The local API binds 127.0.0.1 inside the container; publish it through
# `docker run -p 7878:7878` (loopback on the host) exactly like the app does.
EXPOSE 7878

ENTRYPOINT ["/usr/local/bin/partstable", "serve"]

FROM scratch

ARG SUITE_ID=unknown
ARG SUITE_TITLE=unknown
ARG SUITE_KIND=suite
ARG SUITE_VERSION=latest

LABEL org.opencontainers.image.title="${SUITE_TITLE}" \
      org.opencontainers.image.version="${SUITE_VERSION}" \
      org.opencontainers.image.description="Seed artifact for local BabelSuite Zot testing" \
      io.babelsuite.id="${SUITE_ID}" \
      io.babelsuite.kind="${SUITE_KIND}"

COPY artifact.txt /artifact.txt

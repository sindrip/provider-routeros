FROM alpine:3.24.1

RUN apk --no-cache add ca-certificates

ARG TARGETOS
ARG TARGETARCH

COPY "bin/${TARGETOS}_${TARGETARCH}/provider" /usr/local/bin/provider

USER 65532

EXPOSE 8081 8082

ENTRYPOINT ["provider"]

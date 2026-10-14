# The stock Oiko, its built-in types only, published as ghcr.io/llehouerou/oiko (ADR 0057).
# Build it from a release tag's tree, which carries web/dist, the stamped version and
# THIRD_PARTY_NOTICES.md (ADR 0018). The runtime stage is the catalogue's Dockerfile's, so a home
# moving to a build adding types keeps its /data.
FROM --platform=$BUILDPLATFORM golang:1.27.1 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /oiko ./cmd/oiko

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /oiko /oiko
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /
ENV OIKO_INSTALL=image
VOLUME /data
ENTRYPOINT ["/oiko", "-data", "/data"]

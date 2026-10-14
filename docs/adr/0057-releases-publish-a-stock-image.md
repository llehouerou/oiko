# Releases publish a stock image

Each release of Oiko publishes a Docker image of the stock Oiko, its built-in types only (ADR 0021), as `ghcr.io/llehouerou/oiko`, for `linux/amd64` and `linux/arm64`. `.github/workflows/image.yml` builds it when a release tag is pushed, or by hand for a tag given, from the release's tree, which carries the web client and the stamped version (ADR 0018); a commit of `main` is never published. It first runs the image and waits for the dashboard to answer, then pushes it with its provenance and SBOM. An image is tagged `X.Y.Z` and `X.Y`, the release's version without its `v`, never `latest`.

The image sets `OIKO_INSTALL=image`, an Install of its own: `docker` keeps its meaning, an Oiko built from the catalogue's Dockerfile with `oiko-build`. An `image` Oiko is upgraded by pulling the newer release's tag and recreating its container, which the dashboard tells and `oiko upgrade` refuses to do. The new value is an addition, so it ships in a minor release (ADR 0019).

Its layout is the runtime stage of the catalogue's Dockerfile: `gcr.io/distroless/static-debian12:nonroot`, the executable at `/oiko`, `VOLUME /data`, `ENTRYPOINT ["/oiko", "-data", "/data"]`. A home moving between the image and a Dockerfile adding types keeps its `/data`. The image also holds Oiko's `LICENSE`, `NOTICE` and the release's `THIRD_PARTY_NOTICES.md` (ADR 0047). Host networking is the recommended mode, since HomeKit finds accessories over mDNS.

This realises what ADR 0021 assumed when it spoke of the stock binary and Docker image: until now the only image was the one a user built from the catalogue's Dockerfile.

## Considered Options

- **A `latest` tag**: what most images offer, but it crosses a breaking release unnoticed, where Oiko's banner never offers one (ADR 0019). `X.Y` follows only the releases that cannot break a v0 home.
- **A `-builder` image** with Go and `oiko-build`, as Caddy ships: a Dockerfile adding types would start from it, but the catalogue's Dockerfile already does the job from `golang`, and a second image doubles what each release publishes.
- **`linux/arm/v7`**: a 32-bit Raspberry Pi OS would run it, at the cost of a third platform to publish and nothing to test it on.
- **Keeping `docker` and telling the image from a Dockerfile by the Build**, an image being an Oiko with no added type: no new value, but an Oiko built from the catalogue's Dockerfile with no type picked would be told to pull an image it does not run, and the banner would guess from what is compiled in rather than from how Oiko runs.
- **Signing the image with cosign**, **publishing to Docker Hub**, **a healthcheck**: each is more to maintain, and none is asked for yet; the attestations already record how the image was built.

## Consequences

- The image's name, its tags, `/data`, port 8080 through `-listen`'s default, and `OIKO_INSTALL=image` are part of what a release promises: changing one is breaking (ADR 0019).
- The Dockerfile's Go version follows `go.mod`'s, which a test holds them to: a release whose image fails to build cannot be built again from its tag.
- An image is published only if its release's workflow run succeeds; `workflow_dispatch` publishes a tag again after a fix to the workflow, from the tag's own tree.
- A home with other types still builds its own image from the catalogue's Dockerfile.

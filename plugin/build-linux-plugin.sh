#!/usr/bin/env sh
set -eu

out_dir="dist"
mkdir -p "${out_dir}"

docker buildx build \
    --platform linux/arm64 \
    --build-arg "GO_TARBALL_SHA256=${GO_TARBALL_SHA256:-}" \
    --target artifact \
    --output "type=local,dest=${out_dir}" \
    -f Dockerfile.build \
    .

file "${out_dir}/out_stackdriver_wif.so"

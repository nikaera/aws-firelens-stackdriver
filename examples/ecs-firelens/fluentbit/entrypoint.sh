#!/bin/sh
set -eu

exec /fluent-bit/bin/fluent-bit "$@"

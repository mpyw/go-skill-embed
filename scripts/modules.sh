#!/usr/bin/env bash
# The submodules this repository publishes, as directories relative to the root.
#
# The root module is published under the repository's own path and has no
# directory prefix, so it is not listed. examples/ is not published: it exists
# to be read and to be tested, and its go.mod carries a replace that only makes
# sense inside a checkout.
#
# Go tags a submodule as <dir>/<version>, so this list is also the tag list.

set -euo pipefail

find . -mindepth 2 -name go.mod -not -path './examples/*' -exec dirname {} \; \
  | sed 's|^\./||' \
  | sort

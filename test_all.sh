#!/usr/bin/env bash

set -o pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

# Every tool is pinned in mise.toml and reached through the PATH mise sets, so
# local and CI run the same binaries.

declare -a failed_tests=()

run_test() {
    local name="$1"
    shift

    echo "=== $name ==="
    "$@"
    local status=$?
    if [ $status -eq 0 ]; then
        echo -e "${GREEN}[$name] OK${NC}"
    else
        echo -e "${RED}[$name] FAILED${NC}"
        failed_tests+=("$name")
    fi
    return $status
}

echo ""

# The adapters and the example are modules of their own, so that embedding
# skills never drags cobra or x/tools into a consumer's module graph. Nothing
# reaches them unless each one is entered.
modules() {
    find . -name go.mod -not -path './.git/*' -exec dirname {} \; | sort
}

in_each_module() {
    local failed=0
    for dir in $(modules); do
        echo "--- $dir"
        ( cd "$dir" && "$@" ) || failed=1
    done
    return $failed
}

run_test "test" \
    in_each_module go test ./...

## go.mod's toolchain and mise.toml's go say the same thing in two places,
## which is the cost of pinning Go with mise. golangci-lint refuses to load a
## module whose go directive is newer than the Go it was built with, so a drift
## here surfaces as an unrelated-looking lint failure. Check it first instead.
run_test "toolchain" \
    bash -c '
      mod=$(sed -n "s/^toolchain go//p" go.mod)
      mise=$(sed -n "s/^go = \"\(.*\)\"/\1/p" mise.toml)
      if [ "$mod" != "$mise" ]; then
        echo "go.mod toolchain=$mod but mise.toml go=$mise" >&2
        exit 1
      fi
      echo "toolchain $mod"
    '

run_test "lint" \
    in_each_module golangci-lint run ./...

# A failed build reports zero declscope diagnostics and looks exactly like a
# clean one, so the build runs first and the count is only read after it passes.
run_test "declscope" \
    in_each_module bash -c 'go build ./... && declscope ./...'

# A Go block in a document is hand written, so it drifts when a signature
# changes and nothing says so.
run_test "checkdocs" \
    python3 scripts/checkdocs.py

echo ""
echo "===== Summary ====="
if [ ${#failed_tests[@]} -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Failed tests:${NC}"
    for test in "${failed_tests[@]}"; do
        echo -e "  ${RED}- $test${NC}"
    done
    exit 1
fi

#!/usr/bin/env bash
# Note: no -e flag — arithmetic ((x++)) returns exit 1 when value is 0,
# which would terminate the script prematurely with set -e.
set -uo pipefail

BUILD_DIR="build"
PASS=0
FAIL=0

check_binary() {
    local path="$1"
    local expected_os="$2"
    local expected_arch="$3"

    if [[ ! -f "$path" ]]; then
        echo "FAIL: $path does not exist"
        FAIL=$((FAIL + 1))
        return
    fi

    local size
    size=$(stat -c%s "$path" 2>/dev/null || stat -f%z "$path" 2>/dev/null)
    if [[ "$size" -lt 1000000 ]]; then
        echo "FAIL: $path is suspiciously small (${size} bytes)"
        FAIL=$((FAIL + 1))
        return
    fi

    local file_output
    file_output=$(file "$path")

    case "$expected_os" in
        linux)
            if [[ "$file_output" != *"ELF"* ]]; then
                echo "FAIL: $path is not an ELF binary: $file_output"
                FAIL=$((FAIL + 1))
                return
            fi
            ;;
        darwin)
            if [[ "$file_output" != *"Mach-O"* ]]; then
                echo "FAIL: $path is not a Mach-O binary: $file_output"
                FAIL=$((FAIL + 1))
                return
            fi
            ;;
        windows)
            if [[ "$file_output" != *"PE32"* ]] && [[ "$file_output" != *"PE32+"* ]]; then
                echo "FAIL: $path is not a PE binary: $file_output"
                FAIL=$((FAIL + 1))
                return
            fi
            ;;
    esac

    echo "PASS: $path ($expected_os/$expected_arch, ${size} bytes)"
    PASS=$((PASS + 1))
}

echo "=== Cross-Compilation Verification ==="
echo ""

check_binary "$BUILD_DIR/linux-amd64/trasker-client"       linux   amd64
check_binary "$BUILD_DIR/linux-arm64/trasker-client"       linux   arm64
check_binary "$BUILD_DIR/darwin-amd64/trasker-client"      darwin  amd64
check_binary "$BUILD_DIR/darwin-arm64/trasker-client"      darwin  arm64
check_binary "$BUILD_DIR/windows-amd64/trasker-client.exe" windows amd64

echo ""
echo "Results: $PASS passed, $FAIL failed"

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi

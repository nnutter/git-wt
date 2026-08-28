#!/usr/bin/env bash

set -euo pipefail

plugin_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly plugin_root
test_directory="$(mktemp -d)"
readonly test_directory
readonly fake_bin_directory="$test_directory/bin"

cleanup() {
    rm -rf "$test_directory"
}

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    exit 1
}

assert_file_equals() {
    local expected="$1"
    local actual_file="$2"
    local actual

    actual=$(<"$actual_file")
    [[ "$actual" == "$expected" ]] || fail "Unexpected content in $actual_file"
}

assert_file_contains() {
    local expected="$1"
    local actual_file="$2"

    grep -Fq "$expected" "$actual_file" || fail "Missing '$expected' in $actual_file"
}

install_fake_commands() {
    mkdir -p "$fake_bin_directory"

    cat >"$fake_bin_directory/timber" <<'FAKE_TIMBER'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$@" >"$CREATE_ARGUMENTS_FILE"
if [[ "${1:-} ${2:-}" == "tui --herdr" ]]; then
    [[ "${FAKE_CREATE_FAILURE:-}" != "1" ]] || exit 1
    exit 0
fi
exit 1
FAKE_TIMBER

    chmod +x "$fake_bin_directory/timber"
}

run_create_command() (
    cd "$plugin_root"
    env \
        PATH="$fake_bin_directory:/usr/bin:/bin" \
        CREATE_ARGUMENTS_FILE="$test_directory/create-arguments" \
        /bin/bash bin/create
)

test_invokes_tui_create_with_herdr() {
    run_create_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"

    assert_file_equals $'tui\n--herdr' \
        "$test_directory/create-arguments"
}

test_reports_create_failure() {
    rm -f "$test_directory/create-arguments"

    if printf '\n' | FAKE_CREATE_FAILURE=1 run_create_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"; then
        fail 'Create failure returned success'
    fi

    assert_file_equals $'tui\n--herdr' \
        "$test_directory/create-arguments"
    assert_file_contains 'timber could not create or open the worktree.' "$test_directory/stderr"
}

main() {
    trap cleanup EXIT
    install_fake_commands
    test_invokes_tui_create_with_herdr
    test_reports_create_failure
    printf 'All plugin tests passed.\n'
}

main "$@"

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

    cat >"$fake_bin_directory/git-wt" <<'FAKE_GIT_WT'
#!/usr/bin/env bash
set -eu
if [[ "$1 $2 $3" == "repo list --quiet" ]]; then
    [[ "${FAKE_LIST_FAILURE:-}" != "1" ]] || exit 1
    printf 'alpha\nbeta\n'
    exit 0
fi
printf '%s\n' "$@" >"$CREATE_ARGUMENTS_FILE"
FAKE_GIT_WT

    cat >"$fake_bin_directory/fzf" <<'FAKE_FZF'
#!/usr/bin/env bash
set -eu
cat >/dev/null
[[ "${FAKE_FZF_CANCEL:-}" != "1" ]] || exit 130
printf '%s\n' "${FAKE_FZF_SELECTION:-alpha}"
FAKE_FZF

    chmod +x "$fake_bin_directory/git-wt" "$fake_bin_directory/fzf"
}

run_create_command() (
    cd "$plugin_root"
    env \
        PATH="$fake_bin_directory:/usr/bin:/bin" \
        CREATE_ARGUMENTS_FILE="$test_directory/create-arguments" \
        /bin/bash bin/create
)

test_creates_selected_worktree() {
    printf 'topic/name\n' | FAKE_FZF_SELECTION=beta run_create_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"

    assert_file_equals $'create\n--repo\nbeta\ntopic/name' \
        "$test_directory/create-arguments"
}

test_closes_when_selection_is_cancelled() {
    rm -f "$test_directory/create-arguments"

    FAKE_FZF_CANCEL=1 run_create_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"

    [[ ! -e "$test_directory/create-arguments" ]] || fail 'Create ran after cancellation'
}

test_reports_repository_list_failure() {
    rm -f "$test_directory/create-arguments"

    if printf '\n' | FAKE_LIST_FAILURE=1 run_create_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"; then
        fail 'Repository list failure returned success'
    fi

    assert_file_contains 'Could not list git-wt repositories.' "$test_directory/stderr"
}

main() {
    trap cleanup EXIT
    install_fake_commands
    test_creates_selected_worktree
    test_closes_when_selection_is_cancelled
    test_reports_repository_list_failure
    printf 'All plugin tests passed.\n'
}

main "$@"

#!/bin/bash
#
# tests for orx-ralphex-review
# run from any directory: bash scripts/orx-ralphex-review_test.sh
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/orx-ralphex-review"

PASS=0
FAIL=0

ok()  { echo "PASS: $1"; PASS=$((PASS+1)); }
nok() { echo "FAIL: $1"; FAIL=$((FAIL+1)); }

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT INT TERM HUP

MOCKS="$WORK/mocks"
mkdir -p "$MOCKS"

# --- mock: orx (success, writes JSON, leaks progress to stderr) ---
cat > "$MOCKS/orx" <<'MOCK'
#!/bin/bash
echo "orx: querying 2 models" >&2
echo "orx: done" >&2
cat <<'JSON'
{"results":[{"name":"gpt-4o","status":"success","content":"- main.go:10 - potential nil dereference"},{"name":"claude-3","status":"success","content":"- main.go:20 - unused variable"}]}
JSON
MOCK
chmod +x "$MOCKS/orx"

# --- mock: orx that exits 1 (partial failure, valid JSON still produced) ---
cat > "$WORK/orx_partial" <<'MOCK'
#!/bin/bash
echo "orx: model 1 failed" >&2
cat <<'JSON'
{"results":[{"name":"gpt-4o","status":"success","content":"- main.go:10 - issue"},{"name":"claude-3","status":"error","error":"timeout"}]}
JSON
exit 1
MOCK
chmod +x "$WORK/orx_partial"

# --- mock: orx that exits 2 (total failure) ---
cat > "$WORK/orx_total_fail" <<'MOCK'
#!/bin/bash
echo "all models failed" >&2
exit 2
MOCK
chmod +x "$WORK/orx_total_fail"

# --- mock: git that fails on diff ---
cat > "$WORK/git_fail" <<'MOCK'
#!/bin/bash
exit 1
MOCK
chmod +x "$WORK/git_fail"

# --- mock: git (records calls, produces empty diff) ---
GIT_CALLS="$WORK/git_calls.txt"
cat > "$MOCKS/git" <<MOCK
#!/bin/bash
echo "\$*" >> "$GIT_CALLS"
# produce empty diff output (git diff redirected to file in script)
:
MOCK
chmod +x "$MOCKS/git"

# --- helper: init real git repo for tests that need actual git ---
REPO="$WORK/repo"
git init -q "$REPO"
git -C "$REPO" config user.email "test@test.com"
git -C "$REPO" config user.name "Test"
echo "hello" > "$REPO/file.txt"
git -C "$REPO" add file.txt
git -C "$REPO" commit -q -m "initial"

# --- prompt fixtures ---
PROMPT_WITH_MARKER="$WORK/prompt_marker.txt"
cat > "$PROMPT_WITH_MARKER" <<'EOF'
Please review the changes.

Run this command to see the changes:
git diff HEAD~1 HEAD

Review carefully.
EOF

PROMPT_NO_MARKER="$WORK/prompt_no_marker.txt"
cat > "$PROMPT_NO_MARKER" <<'EOF'
PREVIOUS REVIEW CONTEXT
Please dismiss or confirm issues.
EOF

PROMPT_BAD_CMD="$WORK/prompt_bad_cmd.txt"
cat > "$PROMPT_BAD_CMD" <<'EOF'
Run this command to see the changes:
rm -rf /

Review carefully.
EOF

PROMPT_SHORT_CMD="$WORK/prompt_short_cmd.txt"
cat > "$PROMPT_SHORT_CMD" <<'EOF'
Run this command to see the changes:
git

Review carefully.
EOF

PROMPT_OUTPUT_FLAG="$WORK/prompt_output_flag.txt"
cat > "$PROMPT_OUTPUT_FLAG" <<'EOF'
Run this command to see the changes:
git diff --output=/tmp/evil HEAD~1 HEAD

Review carefully.
EOF

PROMPT_EXT_DIFF_FLAG="$WORK/prompt_ext_diff_flag.txt"
cat > "$PROMPT_EXT_DIFF_FLAG" <<'EOF'
Run this command to see the changes:
git diff --ext-diff HEAD~1 HEAD

Review carefully.
EOF

# --- TEST 1: no arguments -> exit 1 ---
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" 2>/dev/null || actual=$?
if [[ "$actual" -eq 1 ]]; then ok "no args -> exit 1"
else nok "no args -> exit 1 (got $actual)"; fi

# --- TEST 2: too many arguments -> exit 1 ---
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" a b 2>/dev/null || actual=$?
if [[ "$actual" -eq 1 ]]; then ok "extra args -> exit 1"
else nok "extra args -> exit 1 (got $actual)"; fi

# --- TEST 3: non-existent prompt file -> exit 1 ---
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$WORK/nonexistent.txt" 2>/dev/null || actual=$?
if [[ "$actual" -eq 1 ]]; then ok "missing file -> exit 1"
else nok "missing file -> exit 1 (got $actual)"; fi

# --- TEST 4: missing jq -> clear error message ---
JQ_PATH="$(command -v jq 2>/dev/null || true)"
if [[ -n "$JQ_PATH" ]]; then
    # build PATH without jq's directory
    JQ_DIR="$(dirname "$JQ_PATH")"
    PATH_NO_JQ=$(echo "$PATH" | tr ':' '\n' | grep -v "^${JQ_DIR}$" | tr '\n' ':' | sed 's/:$//')
    err_output=""
    actual=0
    err_output=$(PATH="$PATH_NO_JQ" "$SCRIPT" "$PROMPT_WITH_MARKER" 2>&1) || actual=$?
    if [[ "$actual" -eq 1 ]] && echo "$err_output" | grep -q "jq is required"; then
        ok "missing jq -> clear error"
    else
        nok "missing jq -> clear error (exit=$actual, output: $err_output)"
    fi
else
    echo "SKIP: jq not installed, skipping missing-jq test"
fi

# --- TEST 5: invalid diff command (not git diff) -> exit 2 ---
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_BAD_CMD" 2>/dev/null || actual=$?
if [[ "$actual" -eq 2 ]]; then ok "invalid diff cmd -> exit 2"
else nok "invalid diff cmd -> exit 2 (got $actual)"; fi

# --- TEST 6: short command (just 'git') -> exit 2 ---
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_SHORT_CMD" 2>/dev/null || actual=$?
if [[ "$actual" -eq 2 ]]; then ok "short cmd (git only) -> exit 2"
else nok "short cmd (git only) -> exit 2 (got $actual)"; fi

# --- TEST 7: --output= flag in diff command -> exit 2 ---
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_OUTPUT_FLAG" 2>/dev/null || actual=$?
if [[ "$actual" -eq 2 ]]; then ok "--output= flag -> exit 2"
else nok "--output= flag -> exit 2 (got $actual)"; fi

# --- TEST 7b: --ext-diff flag in diff command -> exit 2 ---
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_EXT_DIFF_FLAG" 2>/dev/null || actual=$?
if [[ "$actual" -eq 2 ]]; then ok "--ext-diff flag -> exit 2"
else nok "--ext-diff flag -> exit 2 (got $actual)"; fi

# --- TEST 8: diff extraction - git called with extracted args ---
rm -f "$GIT_CALLS"
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_WITH_MARKER" 2>/dev/null || actual=$?
# orx is mocked (not real), exit may be 0 or 4 depending on jq parse; just check git was called
if [[ -f "$GIT_CALLS" ]] && grep -q "HEAD~1 HEAD" "$GIT_CALLS"; then
    ok "diff extraction -> git called with prompt args"
else
    nok "diff extraction -> git called with prompt args (calls: $(cat "$GIT_CALLS" 2>/dev/null || echo 'none'))"
fi

# --- TEST 9: --no-ext-diff --no-textconv in git call ---
if [[ -f "$GIT_CALLS" ]] && grep -q "\-\-no-ext-diff" "$GIT_CALLS" && grep -q "\-\-no-textconv" "$GIT_CALLS"; then
    ok "git diff uses --no-ext-diff --no-textconv"
else
    nok "git diff uses --no-ext-diff --no-textconv (calls: $(cat "$GIT_CALLS" 2>/dev/null || echo 'none'))"
fi

# --- TEST 10: no marker in prompt -> falls back to 'git diff' ---
rm -f "$GIT_CALLS"
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_NO_MARKER" 2>/dev/null || actual=$?
# git should be called with no extra args beyond -c color.ui=false diff --no-ext-diff --no-textconv
if [[ -f "$GIT_CALLS" ]]; then
    CALL=$(cat "$GIT_CALLS")
    # the fallback is 'git diff', so DIFF_ARGS=("git" "diff"), DIFF_ARGS[@]:2 is empty
    if echo "$CALL" | grep -q "\-\-no-ext-diff"; then
        ok "no marker -> falls back to git diff"
    else
        nok "no marker -> falls back to git diff (calls: $CALL)"
    fi
else
    nok "no marker -> falls back to git diff (git not called)"
fi

# --- TEST 11: orx stderr NOT in stdout ---
# use mock orx that writes to stderr; capture script stdout only
stdout_output=""
stdout_output=$(PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_WITH_MARKER" 2>/dev/null) || true
if echo "$stdout_output" | grep -q "orx: querying"; then
    nok "orx stderr suppressed (leaked to stdout)"
else
    ok "orx stderr suppressed from stdout"
fi

# --- TEST 12: orx partial failure (exit 1) -> still produces formatted output ---
# swap orx mock for partial-fail version
cp "$MOCKS/orx" "$WORK/orx_good_backup"
cp "$WORK/orx_partial" "$MOCKS/orx"
output=""
actual=0
output=$(PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_WITH_MARKER" 2>/dev/null) || actual=$?
cp "$WORK/orx_good_backup" "$MOCKS/orx"
if [[ "$actual" -eq 0 ]] && echo "$output" | grep -q "=== gpt-4o ===" && echo "$output" | grep -q "=== claude-3 \[ERROR\] ==="; then
    ok "orx partial failure -> formatted output produced"
else
    nok "orx partial failure -> formatted output produced (exit=$actual, output=$output)"
fi

# --- TEST 13: output format is plain text, not JSON ---
output=""
output=$(PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_WITH_MARKER" 2>/dev/null) || true
if echo "$output" | grep -q "^=== " && ! echo "$output" | grep -q '"results"'; then
    ok "output format is plain text (not JSON)"
else
    nok "output format is plain text (not JSON) (output=$output)"
fi

# --- TEST 14: output includes model name headers ---
if echo "$output" | grep -q "=== gpt-4o ===" && echo "$output" | grep -q "=== claude-3 ==="; then
    ok "output contains model name headers"
else
    nok "output contains model name headers (output=$output)"
fi

# --- TEST 15: empty diff case (no marker) works without error ---
# already covered by test 9, but verify exit 0
rm -f "$GIT_CALLS"
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_NO_MARKER" 2>/dev/null || actual=$?
if [[ "$actual" -eq 0 ]]; then
    ok "empty diff (no marker) -> exit 0"
else
    nok "empty diff (no marker) -> exit 0 (got $actual)"
fi

# --- TEST 16: orx total failure (exit 2) -> exit 4 ---
cp "$MOCKS/orx" "$WORK/orx_good_backup"
cp "$WORK/orx_total_fail" "$MOCKS/orx"
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_WITH_MARKER" 2>/dev/null || actual=$?
cp "$WORK/orx_good_backup" "$MOCKS/orx"
if [[ "$actual" -eq 4 ]]; then
    ok "orx total failure -> exit 4"
else
    nok "orx total failure -> exit 4 (got $actual)"
fi

# --- TEST 17: git diff failure -> exit 3 ---
cp "$MOCKS/git" "$WORK/git_good_backup"
cp "$WORK/git_fail" "$MOCKS/git"
actual=0
PATH="$MOCKS:$PATH" "$SCRIPT" "$PROMPT_WITH_MARKER" 2>/dev/null || actual=$?
cp "$WORK/git_good_backup" "$MOCKS/git"
if [[ "$actual" -eq 3 ]]; then
    ok "git diff failure -> exit 3"
else
    nok "git diff failure -> exit 3 (got $actual)"
fi

# --- summary ---
echo ""
echo "Results: $PASS passed, $FAIL failed"
if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi

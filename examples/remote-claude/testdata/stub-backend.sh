#!/bin/sh
# stub-backend.sh — a deterministic stand-in for the Claude Code backend, used by
# the workspace-bound session example tests so they run with no real Claude and no
# Postgres.
#
# Contract (matches the real backend invocation in
# libs/client/runner/interactive_session.go):
#   - The turn/task text arrives on STDIN (never as an argv argument), proving the
#     stdin-not-argv injection-safety invariant.
#   - The process runs with its CWD set to the bound workspace, so writing a file
#     to a relative path lands the artifact in the workspace and is readable back
#     via workspacefs.NewLocal.
#
# It reads the task from stdin and writes a fixed marker plus the task into
# agent-output.txt in the current directory. Output is deterministic so the tests
# can assert exact bytes.
set -eu

task=$(cat)

printf 'stub-backend ran; task=%s\n' "$task" | tee agent-output.txt

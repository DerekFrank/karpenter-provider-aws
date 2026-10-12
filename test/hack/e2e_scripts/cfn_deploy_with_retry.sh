#!/usr/bin/env bash
# Runs `aws cloudformation deploy --stack-name <stack> <args...>`, retrying with exponential backoff.
# ExecuteChangeSet throttling ("Concurrent resource level operations limit exceeded") is transient when many
# E2E clusters are created at once, so retry instead of failing the whole suite.
# Usage: cfn_deploy_with_retry.sh <stack-name> [aws cloudformation deploy args...]
set -euo pipefail

STACK_NAME="$1"
shift
MAX_ATTEMPTS="${CFN_DEPLOY_MAX_ATTEMPTS:-5}"
BASE_DELAY_SECONDS="${CFN_DEPLOY_BASE_DELAY_SECONDS:-30}"

stack_status() {
  aws cloudformation describe-stacks --stack-name "$STACK_NAME" --query 'Stacks[0].StackStatus' --output text 2>/dev/null || echo "NONE"
}

# A failed create leaves the stack in a state that blocks the next deploy, so wait out any operation still in
# flight and delete the stack if it never got created.
prepare_retry() {
  local status polls=0
  status=$(stack_status)
  while [[ "$status" == *_IN_PROGRESS && "$status" != "REVIEW_IN_PROGRESS" ]] && (( polls++ < 60 )); do
    echo "Stack $STACK_NAME is $status, waiting for it to settle"
    sleep 15
    status=$(stack_status)
  done
  case "$status" in
    REVIEW_IN_PROGRESS|ROLLBACK_COMPLETE)
      echo "Stack $STACK_NAME is $status, deleting it before retrying"
      aws cloudformation delete-stack --stack-name "$STACK_NAME"
      aws cloudformation wait stack-delete-complete --stack-name "$STACK_NAME"
      ;;
  esac
}

for (( attempt = 1; attempt <= MAX_ATTEMPTS; attempt++ )); do
  rc=0
  aws cloudformation deploy --stack-name "$STACK_NAME" "$@" || rc=$?
  if (( rc == 0 )); then
    exit 0
  fi
  if (( attempt == MAX_ATTEMPTS )); then
    echo "::error::aws cloudformation deploy of $STACK_NAME failed after $MAX_ATTEMPTS attempts (exit $rc)"
    exit "$rc"
  fi
  delay=$(( BASE_DELAY_SECONDS * (1 << (attempt - 1)) + RANDOM % (BASE_DELAY_SECONDS + 1) ))
  echo "::warning::aws cloudformation deploy of $STACK_NAME failed (attempt $attempt/$MAX_ATTEMPTS, exit $rc), retrying in ${delay}s"
  sleep "$delay"
  prepare_retry
done

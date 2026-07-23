#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
# shellcheck source=lib/env-file.sh
source "$REPO_ROOT/scripts/lib/env-file.sh"

usage() {
  echo "usage: $0 [--if-present] ENV_FILE -- COMMAND [ARG ...]" >&2
}

optional=false
if [[ "${1:-}" == "--if-present" ]]; then
  optional=true
  shift
fi

if [[ $# -lt 3 || "${2:-}" != "--" || -z "${1:-}" ]]; then
  usage
  exit 64
fi

env_file="$1"
shift 2

if [[ ! -f "$env_file" ]]; then
  if [[ "$optional" == "true" ]]; then
    exec "$@"
  fi
  echo "Open CRM environment file not found: $env_file" >&2
  exit 66
fi

# Validate every assignment before loading anything. Only names are retained;
# values are never evaluated, echoed, or passed through a shell parser.
declare -a env_names=()
line_number=0
while IFS= read -r env_line || [[ -n "$env_line" ]]; do
  line_number=$((line_number + 1))
  env_line="${env_line%$'\r'}"
  env_line="${env_line#"${env_line%%[![:space:]]*}"}"
  [[ -z "$env_line" || "$env_line" == \#* ]] && continue
  if [[ "$env_line" =~ ^([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*= ]]; then
    env_name="${BASH_REMATCH[1]}"
    env_name_seen=false
    for existing_name in "${env_names[@]}"; do
      if [[ "$existing_name" == "$env_name" ]]; then
        env_name_seen=true
        break
      fi
    done
    if [[ "$env_name_seen" == "false" ]]; then
      env_names+=("$env_name")
    fi
    continue
  fi
  echo "Invalid environment assignment at $env_file:$line_number" >&2
  exit 65
done < "$env_file"

if ((${#env_names[@]} > 0)); then
  open_crm_load_env_keys "$env_file" "${env_names[@]}"
fi

exec "$@"

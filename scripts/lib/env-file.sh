#!/usr/bin/env bash

# Load an explicit allowlist of deployment settings from a Compose-style env
# file without executing the file as shell code. Values are treated literally;
# surrounding single or double quotes and unquoted inline comments are removed.
open_crm_load_env_keys() {
  local env_file="$1"
  shift

  [[ -f "$env_file" ]] || return 1

  local requested_name env_line parsed_name parsed_value found first last
  for requested_name in "$@"; do
    [[ "$requested_name" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || return 1
    if [[ -v "$requested_name" ]]; then
      continue
    fi

    found=false
    parsed_value=""
    while IFS= read -r env_line || [[ -n "$env_line" ]]; do
      env_line="${env_line%$'\r'}"
      env_line="${env_line#"${env_line%%[![:space:]]*}"}"
      [[ -z "$env_line" || "$env_line" == \#* ]] && continue
      if [[ "$env_line" =~ ^([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=(.*)$ ]]; then
        parsed_name="${BASH_REMATCH[1]}"
        [[ "$parsed_name" == "$requested_name" ]] || continue
        parsed_value="${BASH_REMATCH[2]}"
        parsed_value="${parsed_value#"${parsed_value%%[![:space:]]*}"}"
        parsed_value="${parsed_value%"${parsed_value##*[![:space:]]}"}"
        if [[ ${#parsed_value} -ge 2 ]]; then
          first="${parsed_value:0:1}"
          last="${parsed_value: -1}"
          if [[ "$first" == "'" && "$last" == "'" ]] || [[ "$first" == '"' && "$last" == '"' ]]; then
            parsed_value="${parsed_value:1:${#parsed_value}-2}"
          elif [[ "$parsed_value" =~ ^(.*[^[:space:]])[[:space:]]+\#.*$ ]]; then
            parsed_value="${BASH_REMATCH[1]}"
          fi
        fi
        found=true
      fi
    done < "$env_file"

    if [[ "$found" == "true" ]]; then
      printf -v "$requested_name" '%s' "$parsed_value"
      export "$requested_name"
    fi
  done
}

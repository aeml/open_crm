#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
runner="$REPO_ROOT/scripts/run-with-env.sh"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/open-crm-dev-env-test.XXXXXX")"
cleanup() {
  rm -rf -- "$test_root"
}
trap cleanup EXIT

literal_marker="$test_root/literal-was-executed"
printf '%s\n' \
  'SPACE_VALUE=Open CRM' \
  "LITERAL_VALUE=\$(touch $literal_marker)" \
  'HASH_VALUE=value#kept' \
  'EQUAL_VALUE=a=b=c' \
  'OVERRIDE_VALUE=file' \
  'EMPTY_OVERRIDE_VALUE=file' \
  'EMPTY_VALUE=' \
  'QUOTED_VALUE="quoted # kept"' \
  'INLINE_VALUE=value # removed' > "$test_root/literal.env"

loaded_values="$(
  env -u SPACE_VALUE -u LITERAL_VALUE -u HASH_VALUE -u EQUAL_VALUE \
    -u EMPTY_VALUE -u QUOTED_VALUE -u INLINE_VALUE \
    OVERRIDE_VALUE=process EMPTY_OVERRIDE_VALUE= "$runner" "$test_root/literal.env" -- \
    bash -c 'printf "%s\n" "$SPACE_VALUE" "$LITERAL_VALUE" "$HASH_VALUE" "$EQUAL_VALUE" "$OVERRIDE_VALUE" "$EMPTY_OVERRIDE_VALUE" "$EMPTY_VALUE" "$QUOTED_VALUE" "$INLINE_VALUE"'
)"
expected_values="$(printf '%s\n' \
  'Open CRM' \
  "\$(touch $literal_marker)" \
  'value#kept' \
  'a=b=c' \
  'process' \
  '' \
  '' \
  'quoted # kept' \
  'value')"
if [[ "$loaded_values" != "$expected_values" ]]; then
  echo "development env loader did not preserve literal values and overrides" >&2
  exit 1
fi
if [[ -e "$literal_marker" ]]; then
  echo "development env loader executed a literal environment value" >&2
  exit 1
fi

printf '%s\n' 'GOOD=value' 'this is not an assignment' > "$test_root/malformed.env"
malformed_marker="$test_root/malformed-command-ran"
if "$runner" "$test_root/malformed.env" -- touch "$malformed_marker" 2>/dev/null; then
  echo "development env loader accepted a malformed line" >&2
  exit 1
fi
if [[ -e "$malformed_marker" ]]; then
  echo "development env loader ran a command after malformed input" >&2
  exit 1
fi

optional_value="$(OPTIONAL_VALUE=preserved "$runner" --if-present "$test_root/missing.env" -- bash -c 'printf %s "$OPTIONAL_VALUE"')"
if [[ "$optional_value" != "preserved" ]]; then
  echo "optional development env fallback changed the process environment" >&2
  exit 1
fi

unset_names=(
  GO_ENV DATABASE_URL VITE_API_BASE_URL BILLING_PROVIDER EMAIL_PROVIDER
  POSTMARK_SERVER_TOKEN POSTMARK_FROM_EMAIL TELEPHONY_PROVIDER CALENDAR_PROVIDER
  SSO_PROVIDER AI_PROVIDER STORAGE_PROVIDER
)
unset_args=()
for unset_name in "${unset_names[@]}"; do
  unset_args+=(-u "$unset_name")
done
example_values="$(
  env "${unset_args[@]}" "$runner" "$REPO_ROOT/example.env" -- \
    bash -c 'printf "%s\n" "$GO_ENV" "$DATABASE_URL" "$VITE_API_BASE_URL" "$BILLING_PROVIDER" "$EMAIL_PROVIDER" "$POSTMARK_SERVER_TOKEN" "$POSTMARK_FROM_EMAIL" "$TELEPHONY_PROVIDER" "$CALENDAR_PROVIDER" "$SSO_PROVIDER" "$AI_PROVIDER" "$STORAGE_PROVIDER"'
)"
expected_example_values="$(printf '%s\n' \
  'development' \
  'postgres://open_crm:open_crm@localhost:5432/open_crm?sslmode=disable' \
  'http://localhost:8080' \
  'fake' \
  'fake' \
  '' \
  '' \
  'fake' \
  'fake' \
  'fake' \
  'fake' \
  'fake')"
if [[ "$example_values" != "$expected_example_values" ]]; then
  echo "example.env is not the promised credential-free local contract" >&2
  exit 1
fi

# Exercise the Makefile boundary without starting Go or touching PostgreSQL.
mkdir -p "$test_root/bin"
make_capture="$test_root/make-capture"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'printf "%s\n%s\n" "$MAKE_ENV_VALUE" "$*" > "$MAKE_CAPTURE"' > "$test_root/bin/go"
chmod +x "$test_root/bin/go"
printf '%s\n' 'MAKE_ENV_VALUE=loaded-by-make' > "$test_root/make.env"
PATH="$test_root/bin:$PATH" MAKE_CAPTURE="$make_capture" \
  make --no-print-directory --silent -C "$REPO_ROOT" \
  ENV_FILE="$test_root/make.env" db-migrate
make_env_value=""
make_args=""
{
  IFS= read -r make_env_value
  IFS= read -r make_args
} < "$make_capture"
if [[ "$make_env_value" != "loaded-by-make" || "$make_args" != "-C apps/api run ./cmd/migrate" ]]; then
  echo "Makefile development target did not load the selected env file exactly" >&2
  exit 1
fi

# The checkout directory and a deployment checkout may share the same basename.
# Exercise the Make boundary without invoking Docker so a future edit cannot let
# local database commands discover or stop a deployment project by accident.
compose_capture="$test_root/compose-capture"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'printf "%s\n" "$*" >> "$COMPOSE_CAPTURE"' > "$test_root/bin/docker"
chmod +x "$test_root/bin/docker"
PATH="$test_root/bin:$PATH" COMPOSE_CAPTURE="$compose_capture" \
  make --no-print-directory --silent -C "$REPO_ROOT" db-up db-down
expected_compose_calls="$(printf '%s\n' \
  'compose --project-name open-crm-dev --file docker-compose.yml up -d postgres' \
  'compose --project-name open-crm-dev --file docker-compose.yml down')"
if [[ "$(< "$compose_capture")" != "$expected_compose_calls" ]]; then
  echo "Makefile database targets are not isolated to the development Compose project" >&2
  exit 1
fi

: > "$compose_capture"
PATH="$test_root/bin:$PATH" COMPOSE_CAPTURE="$compose_capture" \
  make --no-print-directory --silent -C "$REPO_ROOT" \
  DEV_COMPOSE_PROJECT=open-crm-review db-up
if [[ "$(< "$compose_capture")" != \
  'compose --project-name open-crm-review --file docker-compose.yml up -d postgres' ]]; then
  echo "Makefile database target did not preserve an explicit development project override" >&2
  exit 1
fi

compose_name="$(docker compose --file "$REPO_ROOT/docker-compose.yml" config --format json | \
  sed -n 's/^[[:space:]]*"name":[[:space:]]*"\([^"]*\)".*/\1/p' | \
  head -n 1)"
if [[ "$compose_name" != "open-crm-dev" ]]; then
  echo "development Compose file does not declare the isolated open-crm-dev project" >&2
  exit 1
fi

echo "development_environment_contract_succeeded"

#!/usr/bin/env bash
# Register a machine in Agent Hub from the command line, then verify it.
#
# Exists because the fiddly part of onboarding a host is pasting a multi-line
# PEM key into a browser form. This reads the key from a file instead, and runs
# the same preflight the Machines page does so you learn immediately whether the
# hub can actually reach the host.
#
# The admin password is read interactively and is never echoed, never passed as
# an argument (so it stays out of `ps` and shell history), and never written to
# disk. Set AGENT_HUB_ADMIN_PASSWORD to skip the prompt in automation.
#
# Usage:
#   scripts/register-machine.sh \
#     --base https://your-host.example \
#     --name web1 --address 100.64.0.10 --port 2222 --user ops \
#     --key ~/.ssh/agent-hub_ed25519
#
#   # password-authenticated host instead of a key:
#   scripts/register-machine.sh --base ... --name box --address 10.0.0.5 \
#     --user root --ssh-password
set -euo pipefail

BASE="" NAME="" ADDRESS="" PORT=22 SSH_USER="root" KEY_FILE="" ADMIN="admin"
ASK_SSH_PASSWORD=0

die() { echo "error: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base)         BASE="${2:-}"; shift 2 ;;
    --name)         NAME="${2:-}"; shift 2 ;;
    --address)      ADDRESS="${2:-}"; shift 2 ;;
    --port)         PORT="${2:-}"; shift 2 ;;
    --user)         SSH_USER="${2:-}"; shift 2 ;;
    --key)          KEY_FILE="${2:-}"; shift 2 ;;
    --admin)        ADMIN="${2:-}"; shift 2 ;;
    --ssh-password) ASK_SSH_PASSWORD=1; shift ;;
    -h|--help)      sed -n '2,20p' "$0"; exit 0 ;;
    *)              die "unknown argument: $1" ;;
  esac
done

[[ -n "$BASE"    ]] || die "--base is required (e.g. https://your-host.example)"
[[ -n "$NAME"    ]] || die "--name is required"
[[ -n "$ADDRESS" ]] || die "--address is required"
command -v curl >/dev/null || die "curl is required"
command -v python3 >/dev/null || die "python3 is required (JSON handling)"

BASE="${BASE%/}"

SSH_KEY=""
if [[ -n "$KEY_FILE" ]]; then
  [[ -f "$KEY_FILE" ]] || die "key file not found: $KEY_FILE"
  SSH_KEY="$(cat "$KEY_FILE")"
fi

SSH_PASSWORD=""
if [[ "$ASK_SSH_PASSWORD" == "1" ]]; then
  read -r -s -p "SSH password for ${SSH_USER}@${ADDRESS}: " SSH_PASSWORD; echo
fi

ADMIN_PASSWORD="${AGENT_HUB_ADMIN_PASSWORD:-}"
if [[ -z "$ADMIN_PASSWORD" ]]; then
  read -r -s -p "Agent Hub password for '${ADMIN}': " ADMIN_PASSWORD; echo
fi
[[ -n "$ADMIN_PASSWORD" ]] || die "no password given"

# Build every request body with json.dumps so PEM newlines and shell
# metacharacters survive intact.
json_field() { python3 -c 'import json,sys; print(json.dumps(dict(zip(sys.argv[1::2], sys.argv[2::2]))))' "$@"; }

login_body="$(json_field username "$ADMIN" password "$ADMIN_PASSWORD")"
unset ADMIN_PASSWORD

login_res="$(curl -sS -m 30 -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' --data-binary "$login_body")" || die "login request failed"
unset login_body

TOKEN="$(python3 -c '
import json,sys
try:
    print(json.loads(sys.stdin.read()).get("token",""))
except Exception:
    print("")' <<<"$login_res")"
[[ -n "$TOKEN" ]] || die "login failed: $login_res"
echo "✓ signed in to $BASE"

machine_body="$(python3 - "$NAME" "$ADDRESS" "$PORT" "$SSH_USER" "$SSH_PASSWORD" "$SSH_KEY" <<'PY'
import json, sys
name, address, port, user, password, key = sys.argv[1:7]
body = {
    "name": name, "address": address, "port": int(port),
    "ssh_user": user, "ssh_password": password,
}
if key.strip():
    body["ssh_private_key"] = key
print(json.dumps(body))
PY
)"

create_res="$(curl -sS -m 30 -X POST "$BASE/api/machines" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  --data-binary "$machine_body")" || die "create request failed"

MACHINE_ID="$(python3 -c '
import json,sys
try:
    print(json.loads(sys.stdin.read()).get("id",""))
except Exception:
    print("")' <<<"$create_res")"
[[ -n "$MACHINE_ID" ]] || die "create failed: $create_res"
echo "✓ registered '$NAME' ($SSH_USER@$ADDRESS:$PORT) id=$MACHINE_ID"

echo "→ testing connection…"
check_res="$(curl -sS -m 60 -X POST "$BASE/api/machines/$MACHINE_ID/check" \
  -H "Authorization: Bearer $TOKEN")" || die "check request failed"

# The response goes in as an argument, not on stdin: a heredoc script and a
# here-string payload both claim stdin, and the payload wins — python then
# tries to execute the JSON.
python3 - "$check_res" <<'PY'
import json, sys
try:
    r = json.loads(sys.argv[1])
except Exception as e:
    print("could not parse check response:", e); raise SystemExit(1)
if r.get("ok"):
    print(f"✓ reachable — SSH authenticated in {r.get('elapsed_ms', '?')} ms")
    raise SystemExit(0)
f = r.get("failure") or {}
print(f"✗ {f.get('message', 'connection failed')}  [{f.get('kind', 'unknown')}]")
if f.get("approval_url"):
    print(f"  approve: {f['approval_url']}")
if f.get("hint"):
    print(f"  fix: {f['hint']}")
if f.get("retryable") is False:
    print("  retrying will not help — apply the fix above first")
raise SystemExit(1)
PY

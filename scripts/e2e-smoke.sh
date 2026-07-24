#!/usr/bin/env bash
# End-to-end smoke: login → one machine → multi-session → exec per session.
set -euo pipefail

API="${API_BASE:-http://localhost:27341}"
FE="${FE_BASE:-http://localhost:27342}"
USER="${E2E_USER:-admin}"
PASS="${E2E_PASS:-123456}"
SSH_ADDR="${E2E_SSH_ADDR:-ssh-target}"
SSH_PORT="${E2E_SSH_PORT:-27343}"
SSH_USER="${E2E_SSH_USER:-root}"
SSH_PASS="${E2E_SSH_PASSWORD:-targetpass}"
MARKER="agent-hub-e2e"

echo "== health =="
curl -sfS "$API/health" | tee /dev/stderr | grep -q '"status":"ok"'

echo "== login =="
LOGIN=$(curl -sfS -X POST "$API/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}")
TOKEN=$(echo "$LOGIN" | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')
test -n "$TOKEN"

echo "== unauth protected =="
code=$(curl -sS -o /dev/null -w '%{http_code}' "$API/api/me" || true)
test "$code" = "401"

echo "== ensure single demo machine =="
# delete all existing machines
MACHINES=$(curl -sfS "$API/api/machines" -H "Authorization: Bearer $TOKEN")
echo "$MACHINES" | python3 -c '
import sys,json,urllib.request,os
token=os.environ.get("TOKEN","")
' 2>/dev/null || true
python3 - <<PY
import json, urllib.request
api = "$API"
token = """$TOKEN"""
req = urllib.request.Request(api + "/api/machines", headers={"Authorization": "Bearer " + token})
with urllib.request.urlopen(req) as r:
    machines = json.load(r)["machines"]
for m in machines:
    d = urllib.request.Request(api + "/api/machines/" + m["id"], method="DELETE",
        headers={"Authorization": "Bearer " + token})
    try:
        urllib.request.urlopen(d)
    except Exception as e:
        print("delete", m["id"], e)
print("deleted", len(machines), "machines")
PY

CREATED=$(curl -sfS -X POST "$API/api/machines" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"ssh-dummy\",\"address\":\"$SSH_ADDR\",\"port\":$SSH_PORT,\"ssh_user\":\"$SSH_USER\",\"ssh_password\":\"$SSH_PASS\"}")
echo "$CREATED"
MID=$(echo "$CREATED" | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')

echo "== create two terminal sessions =="
T1=$(curl -sfS -X POST "$API/api/machines/$MID/terminals" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"build"}')
T2=$(curl -sfS -X POST "$API/api/machines/$MID/terminals" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"debug"}')
echo "$T1"
echo "$T2"
TID1=$(echo "$T1" | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')
TID2=$(echo "$T2" | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')
test "$TID1" != "$TID2"

echo "== list sessions =="
LIST=$(curl -sfS "$API/api/machines/$MID/terminals" -H "Authorization: Bearer $TOKEN")
echo "$LIST" | grep -q "$TID1"
echo "$LIST" | grep -q "$TID2"
echo "$LIST" | python3 -c 'import sys,json; assert len(json.load(sys.stdin)["terminals"])==2'

echo "== exec on session (round 1) =="
E1=$(curl -sfS -X POST "$API/api/terminals/$TID1/exec" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"command\":\"echo $MARKER && whoami\"}")
echo "$E1"
echo "$E1" | grep -q "$MARKER"

echo "== exec on session (round 2) =="
E2=$(curl -sfS -X POST "$API/api/terminals/$TID2/exec" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"command\":\"echo $MARKER-s2\"}")
echo "$E2"
echo "$E2" | grep -q "$MARKER-s2"

echo "== close one session =="
curl -sfS -o /dev/null -w "%{http_code}\n" -X DELETE "$API/api/terminals/$TID1" \
  -H "Authorization: Bearer $TOKEN" | grep -q 204
LIST2=$(curl -sfS "$API/api/machines/$MID/terminals" -H "Authorization: Bearer $TOKEN")
echo "$LIST2" | python3 -c 'import sys,json; ts=json.load(sys.stdin)["terminals"]; assert len(ts)==1 and ts[0]["id"]=="'"$TID2"'"'

echo "== FE shell =="
FE="${FE_BASE:-http://localhost:27342}"
curl -sfS -o /dev/null -w "FE HTTP %{http_code}\n" "$FE/"
curl -sfS -o /dev/null -w "workspace HTTP %{http_code}\n" "$FE/workspace" || true
# SPA serves index for all routes
curl -sfS "$FE/" | grep -q 'id="root"'

echo "E2E_SMOKE_OK"

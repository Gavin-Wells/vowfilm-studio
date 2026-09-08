#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ ! -f .env ]]; then echo 'Copy .env.example to .env and configure StarNet and the backend token.' >&2; exit 1; fi
export VOWFILM_ENV_FILE="$PWD/.env"
export VOWFILM_DATA_DIR="$PWD/data"
python3 - <<'PY'
from pathlib import Path
env = dict(line.split('=', 1) for line in Path('.env').read_text().splitlines() if '=' in line and not line.startswith('#'))
Path('.dev.vars').write_text('\n'.join(f'{key}={env.get(key, "")}' for key in ['GO_BACKEND_URL', 'GO_BACKEND_TOKEN']) + '\n')
Path('.dev.vars').chmod(0o600)
PY
(cd server && go build -o bin/vowfilm ./cmd/server)
./server/bin/vowfilm &
backend_pid=$!
trap 'kill "$backend_pid" 2>/dev/null || true' EXIT
npm run dev -- --host 127.0.0.1 --port 5179

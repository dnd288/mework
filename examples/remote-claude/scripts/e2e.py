#!/usr/bin/env python3
"""End-to-end test for the remote-claude example — pure stdlib, black-box.

Drives the real binaries + HTTP API (not the Go libraries) through the full
three-component flow:

    mework server start            (the hub, in-process)
    mework login / runner enroll   (auth + install-once enrollment)
    mework daemon start            (the local runner, SSE-subscribed)
    mework sandbox start -w <ws>   (workspace → long-lived sandbox via dispatch)
    mework session send / attach   (chat turn over the bus → events streamed back)

The agent backend is the deterministic stub (testdata/stub-backend.sh), so the run
needs no real Claude and the assertions are exact. Point the workspace's mework.yml
`backend:` at `claude` to run a real agent instead.

Prerequisites (otherwise the test SKIPs, exit 0):
  - Go toolchain on PATH (to build the binary)
  - A reachable Postgres — set DATABASE_URL, or it defaults to the local test DB
    (run `make test-db` first). The target database is created if `psql` is available.

Run:  cd /path/to/mework && python3 examples/remote-claude/scripts/e2e.py
Exit: 0 = pass or skip, 1 = fail.
"""

import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
EXAMPLE_DIR = Path(__file__).resolve().parents[1]
WORKSPACE_FIXTURE = EXAMPLE_DIR / "testdata" / "workspace"
STUB_BACKEND = EXAMPLE_DIR / "testdata" / "stub-backend.sh"

PAT = "e2e-mello-pat"
MELLO_USER = {"id": "e2e-user-1", "email": "e2e@example.com", "name": "E2E User"}
SERVER_KEY = "e2e-server-key-0123456789"          # >= 16 chars (server fails fast otherwise)
SECRET_KEY = "e2e-secret-key-0123456789"
TENANT_ID = "00000000-0000-0000-0000-000000000001"  # default tenant seeded by migration
TASK = "e2e-marker please summarize"
EXPECTED = f"stub-backend ran; task={TASK}"        # what the stub emits (stout + artifact)


# ---------------------------------------------------------------------------- #
# helpers
# ---------------------------------------------------------------------------- #

def log(msg):
    print(f"[e2e] {msg}", flush=True)


def skip(msg):
    log(f"SKIP: {msg}")
    sys.exit(0)


def fail(msg):
    log(f"FAIL: {msg}")
    sys.exit(1)


def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


def run(cmd, env=None, check=True, capture=True, timeout=60, cwd=None):
    """Run a command, returning (rc, stdout, stderr)."""
    res = subprocess.run(
        cmd, env=env, timeout=timeout, cwd=cwd,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
        text=True,
    )
    out = res.stdout or ""
    err = res.stderr or ""
    if check and res.returncode != 0:
        fail(f"command failed ({res.returncode}): {' '.join(cmd)}\nstdout: {out}\nstderr: {err}")
    return res.returncode, out, err


def http_get(url, headers=None, timeout=5):
    req = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.status, resp.read().decode()


def http_post(url, headers=None, body=b"", timeout=10, check=True):
    req = urllib.request.Request(url, data=body, headers=headers or {}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read().decode()
    except urllib.error.HTTPError as e:
        if check:
            raise
        return e.code, e.read().decode()


def wait_until(predicate, what, timeout=30, interval=0.5):
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        try:
            if predicate():
                return
        except Exception as e:  # noqa: BLE001
            last = e
        time.sleep(interval)
    fail(f"timed out waiting for {what}" + (f" (last error: {last})" if last else ""))


# ---------------------------------------------------------------------------- #
# mock Mello (so PAT auth — server → Mello /me — succeeds without a real Mello)
# ---------------------------------------------------------------------------- #

class _MelloHandler(BaseHTTPRequestHandler):
    def _json(self, obj, status=200):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):  # noqa: N802
        if self.path.rstrip("/").endswith("/me") or self.path == "/me":
            return self._json(MELLO_USER)
        # workspace/board sync calls etc. → benign empties
        return self._json([])

    def do_POST(self):  # noqa: N802
        return self._json({"id": "noop"}, status=201)

    def log_message(self, *_):  # silence
        pass


def start_mock_mello():
    port = free_port()
    httpd = ThreadingHTTPServer(("127.0.0.1", port), _MelloHandler)
    t = threading.Thread(target=httpd.serve_forever, daemon=True)
    t.start()
    return httpd, f"http://127.0.0.1:{port}"


# ---------------------------------------------------------------------------- #
# main
# ---------------------------------------------------------------------------- #

def main():
    if shutil.which("go") is None:
        skip("Go toolchain not on PATH (needed to build the mework binary)")

    dsn = os.environ.get(
        "DATABASE_URL", "postgres://postgres:postgres@localhost:5432/mework_e2e?sslmode=disable"
    )
    ensure_database(dsn)

    tmp = Path(tempfile.mkdtemp(prefix="mework-e2e-"))
    bin_path = tmp / "mework"
    home = tmp / "home"
    home.mkdir()
    workspace = tmp / "workspace"
    procs = {}
    mock = None
    session_id = None
    try:
        # 1. build the single mework binary (CLI + daemon + `server start`)
        log("building mework ...")
        run(["go", "build", "-o", str(bin_path), "./apps/mework"],
            env={**os.environ}, timeout=300, capture=True, cwd=str(REPO_ROOT))
        if not bin_path.exists():
            fail("build did not produce the mework binary")

        # 2. workspace fixture → temp, point backend at the deterministic stub
        shutil.copytree(WORKSPACE_FIXTURE, workspace)
        os.chmod(STUB_BACKEND, 0o755)
        myml = workspace / "mework.yml"
        myml.write_text(myml.read_text().replace("backend: claude", f"backend: {STUB_BACKEND}"))

        # 3. mock Mello + the hub
        mock, mello_url = start_mock_mello()
        hub_port = free_port()
        hub_url = f"http://127.0.0.1:{hub_port}"
        server_env = {
            **os.environ,
            "DATABASE_URL": dsn,
            "SERVER_KEY": SERVER_KEY,
            "MEWORK_SECRET_KEY": SECRET_KEY,
            "MELLO_BASE_URL": mello_url,
            "LISTEN_ADDR": f"127.0.0.1:{hub_port}",
            "MEWORK_HOME": str(home / "server"),
        }
        log(f"starting hub on {hub_url} (db={dsn.split('@')[-1]}) ...")
        procs["server"] = subprocess.Popen([str(bin_path), "server", "start"], env=server_env,
                                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            wait_until(lambda: http_get(f"{hub_url}/readyz")[0] == 200, "hub /readyz", timeout=40)
        except SystemExit:
            # hub never became ready — almost always an unreachable/missing DB
            skip(f"hub did not become ready (is Postgres reachable at {dsn.split('@')[-1]}?)")

        # CLI env: MELLO_BASE_URL → mock (PAT validation), MEWORK_SERVER_URL → hub.
        cli_env = {
            **os.environ,
            "MEWORK_HOME": str(home),
            "MEWORK_SERVER_URL": hub_url,
            "MELLO_BASE_URL": mello_url,
        }

        def mw(*args, **kw):
            return run([str(bin_path), *args], env=cli_env, **kw)

        # 4. log in (PAT), enroll this machine as a runner, start the daemon
        log("login + enroll + daemon ...")
        mw("login", "--token", PAT)

        # Persist server_url to config so the daemon (re-execed child) finds it.
        # The config loader reads from file only, not env vars.  Neither login
        # nor runner enroll writes this field automatically.
        cfg_path = home / "config.json"
        if cfg_path.exists():
            with open(cfg_path) as f:
                cfg = json.load(f)
            cfg["server_url"] = hub_url
            with open(cfg_path, "w") as f:
                json.dump(cfg, f, indent=2)

        _, tok_body = http_post(f"{hub_url}/api/v1/runners/registration-tokens",
                                headers={"Authorization": f"Bearer {PAT}",
                                         "Content-Type": "application/json"},
                                body=json.dumps({"tenant_id": TENANT_ID}).encode())
        reg = json.loads(tok_body).get("token")
        if not reg:
            fail(f"no registration token in response: {tok_body}")
        mw("runner", "enroll", "--url", hub_url, "--token", reg)
        if not (home / "identity.json").exists():
            fail("runner enroll did not persist ~/.mework/identity.json")

        daemon_log = tmp / "daemon.log"
        log("starting daemon (logs -> {}) ...".format(daemon_log))
        with open(daemon_log, "w") as df:
            procs["daemon"] = subprocess.Popen([str(bin_path), "daemon", "start"],
                                               env=cli_env, stdout=df, stderr=subprocess.STDOUT)
        try:
            wait_until(lambda: "running" in mw("daemon", "status", check=False)[1], "daemon running")
        except SystemExit:
            # daemon never showed as running — dump its own log + home dir
            dlog = home / "daemon.log"
            if dlog.exists():
                log("=== daemon.log (%d bytes) ===" % dlog.stat().st_size)
                log(dlog.read_text()[-2000:])
            for f in home.iterdir():
                if f.is_file():
                    log("  home/%s (%d bytes)" % (f.name, f.stat().st_size))
            raise
        time.sleep(2)  # let the SSE subscription establish

        # 5. Register the agent with the hub so DispatchSessionToRunner's
        # agentExists check passes.  The definition payload mirrors what the
        # workspace mework.yml carries (the daemon resolves from the local file).
        log("register agent workspace-agent ...")
        agent_def = {
            "name": "workspace-agent",
            "version": "1.0.0",
            "engine": "local",
            "backend": "claude",
            "author": "mework-examples",
        }
        _, _ = http_post(
            f"{hub_url}/api/v1/agents/workspace-agent/versions",
            headers={"Authorization": f"Bearer {PAT}", "Content-Type": "application/json"},
            body=json.dumps({
                "version": "1.0.0",
                "form": "definition",
                "payload": json.dumps(agent_def),
            }).encode(),
            check=False,  # 409 Conflict means already registered (from prior run)
        )

        # 7. turn the workspace into a running worker
        log("sandbox start -w <workspace> ...")
        _, out, _ = mw("sandbox", "start", "-w", str(workspace), "--json")
        session_id = json.loads(out).get("ID")
        if not session_id:
            fail(f"sandbox start returned no session id: {out}")
        log(f"session = {session_id}")

        # the session must register on the daemon before we send a turn
        wait_until(lambda: session_id in mw("session", "list", "--json", check=False)[1],
                   "session to appear", timeout=20)

        # 8. start the event stream FIRST, THEN send the turn (attach-before-send
        #    ordering is required — events are published on the in-memory broker
        #    with no retention, so the subscriber must already be listening).
        log("attach (background) + send turn ...")
        attach_out_file = tmp / "attach.out"
        with open(attach_out_file, "w") as af:
            attach_proc = subprocess.Popen(
                [str(bin_path), "session", "attach", session_id, "--idle", "8s"],
                env=cli_env, stdout=af, stderr=subprocess.STDOUT, text=True,
            )
        time.sleep(1)  # let the SSE subscription establish

        mw("session", "send", session_id, TASK)

        # Wait for attach to exit (idle timeout or done event)
        try:
            attach_proc.wait(timeout=30)
        except subprocess.TimeoutExpired:
            attach_proc.kill()
        attach_out = attach_out_file.read_text() if attach_out_file.exists() else ""
        log(f"attach done (len={len(attach_out)})")

        # 9. assertions — the deterministic stub output streamed back AND the
        #    artifact landed in the bound workspace on disk
        artifact = workspace / "agent-output.txt"
        try:
            wait_until(lambda: artifact.exists() and EXPECTED in artifact.read_text(),
                       "artifact written into the bound workspace", timeout=15)
        except SystemExit:
            log("=== daemon.log (Popen capture) ===")
            if daemon_log and daemon_log.exists():
                log(daemon_log.read_text()[-3000:])
            dlog = home / "daemon.log"
            if dlog.exists():
                log("=== daemon.log (actual, %d bytes) ===" % dlog.stat().st_size)
                log(dlog.read_text()[-2000:])
            if artifact.exists():
                log(f"artifact CONTENT: {artifact.read_text().strip()}")
            else:
                log("artifact does NOT exist")
            fail("artifact was not produced by the stub backend")
        if EXPECTED not in attach_out:
            dlog = home / "daemon.log"
            if dlog.exists():
                log("=== daemon.log (actual, %d bytes) ===" % dlog.stat().st_size)
                log(dlog.read_text()[-2000:])
            fail(f"attach stream missing expected output {EXPECTED!r}\n--- attach ---\n{attach_out!r}")

        log(f"artifact OK: {artifact.read_text().strip()}")
        log("attach stream carried the worker's output OK")

        # 10. close the worker
        mw("sandbox", "stop", session_id, check=False)
        log("PASS — full server → daemon → sandbox → chat flow verified")
        return 0
    finally:
        # Stop the daemon first via its health endpoint (the Popen below only
        # captures the parent `daemon start` process, which spawns a detached
        # child and exits — killing the Popen would be a no-op).
        try:
            subprocess.run(
                [str(bin_path), "daemon", "stop"],
                env={**os.environ, "MEWORK_HOME": str(home)},
                capture_output=True, timeout=15,
            )
        except Exception:  # noqa: BLE001
            pass
        # Kill all tracked subprocesses.
        for name, p in procs.items():
            try:
                p.send_signal(signal.SIGTERM)
                p.wait(timeout=10)
            except Exception:  # noqa: BLE001
                p.kill()
        if mock is not None:
            mock.shutdown()
        shutil.rmtree(tmp, ignore_errors=True)


def ensure_database(dsn):
    """Best-effort: create the target database if psql is available. The server
    runs migrations on the schema; the database itself must exist."""
    if shutil.which("psql") is None:
        return  # assume the DB exists; the server will migrate or fail → SKIP
    # split "<base>/<dbname>?..." → admin URL on the 'postgres' db + the dbname
    base, _, tail = dsn.rpartition("/")
    dbname = tail.split("?")[0]
    if not base or not dbname:
        return
    admin = f"{base}/postgres"
    subprocess.run(["psql", admin, "-v", "ON_ERROR_STOP=0", "-c", f'CREATE DATABASE "{dbname}"'],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    sys.exit(main())

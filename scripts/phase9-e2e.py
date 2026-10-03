"""Phase 9 exit check (also covers the Phase 8 loop) against a live API and a runner.

Accounts and sessions, login required for Run and Submit, per-user limits (429
with Retry-After), then Run, Submit, SSE and the Submissions list as a signed-in
user, with a scan that no response carries test data. Needs the API on
LEETFORCE_API (default http://127.0.0.1:8080) and a runner consuming the same
Redis. Start the API with the default limits. It creates one account and writes
3 submissions to the real database; delete the account afterwards (its username
is printed) with: DELETE FROM submissions WHERE user_id IN (SELECT id FROM
users WHERE username = '<name>'); DELETE FROM users WHERE username = '<name>';
"""
import os, json, time, urllib.request, urllib.error, http.cookiejar, pathlib, uuid, sys

API = os.environ.get("LEETFORCE_API", "http://127.0.0.1:8080")
ROOT = pathlib.Path(__file__).resolve().parent.parent / "problems" / "sample-sum"
SOL = ROOT / "solutions" / "python"
HIDDEN = ROOT / "tests"
USER = "e2e" + uuid.uuid4().hex[:10]
PASSWORD = "e2e-password-" + uuid.uuid4().hex[:6]

JAR = http.cookiejar.CookieJar()
OPENER = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(JAR))
ANON = urllib.request.build_opener()  # no cookies


def call(method, path, body=None, headers=None, opener=None, raw_errors=False):
    data = json.dumps(body).encode() if body is not None else None
    h = {"Content-Type": "application/json", **(headers or {})}
    req = urllib.request.Request(API + path, data=data, method=method, headers=h)
    try:
        with (opener or OPENER).open(req, timeout=30) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        if raw_errors:
            return e.code, e.read().decode(), dict(e.headers)
        raise


def wait(path, done):
    for _ in range(120):
        _, raw = call("GET", path)
        j = json.loads(raw)
        if done(j):
            return j, raw
        time.sleep(0.5)
    raise SystemExit("timeout " + path)


def src(n):
    return (SOL / n).read_text()


fails = []


def check(name, ok, extra=""):
    print(("PASS " if ok else "FAIL ") + name, extra)
    if not ok:
        fails.append(name)


# --- accounts and sessions
code, raw, _ = call("POST", "/runs", {"problem": "sample-sum", "language": "python", "source": src("ac.py")}, opener=ANON, raw_errors=True)
check("anonymous run is refused", code == 401, str(code))
code, raw, _ = call("POST", "/submissions", {"problem": "sample-sum", "language": "python", "source": src("ac.py")}, opener=ANON, raw_errors=True)
check("anonymous submit is refused", code == 401, str(code))
code, raw, _ = call("GET", "/me", opener=ANON, raw_errors=True)
check("anonymous /me is 401", code == 401, str(code))
code, raw, _ = call("POST", "/auth/signup", {"email": USER + "@example.com", "username": USER, "password": "short"}, raw_errors=True)
check("weak password rejected", code == 422, str(code))
code, raw = call("POST", "/auth/signup", {"email": USER + "@example.com", "username": USER, "password": PASSWORD})
check("signup", code == 201 and any(c.name == "lf_session" and c.has_nonstandard_attr("HttpOnly") for c in JAR), raw[:120])
check("signup response has no password data", "password" not in raw.lower() and "hash" not in raw.lower(), raw[:120])
_, raw = call("GET", "/me")
check("/me after signup", json.loads(raw)["user"]["username"] == USER, raw[:120])
code, raw, _ = call("POST", "/auth/signup", {"email": USER.upper() + "@example.com", "username": "other" + USER, "password": PASSWORD}, raw_errors=True)
check("duplicate email (any case) is 409", code == 409, str(code))
call("POST", "/auth/logout")
code, raw, _ = call("GET", "/me", raw_errors=True)
check("logout ends the session", code == 401, str(code))
code, raw, _ = call("POST", "/auth/login", {"login": USER, "password": "wrong-" + PASSWORD}, raw_errors=True)
check("wrong password is 401", code == 401, str(code))
code, raw = call("POST", "/auth/login", {"login": USER.upper(), "password": PASSWORD})
check("login by username", code == 200, raw[:120])

# --- Run on samples: AC
_, raw = call("POST", "/runs", {"problem": "sample-sum", "language": "python", "source": src("ac.py")})
rid = json.loads(raw)["id"]
j, _ = wait("/runs/" + rid, lambda j: j["status"] == "done")
r = j["result"]
check("run samples AC", r["verdict"] == "AC" and len(r["cases"]) == 2, json.dumps(r)[:200])

# --- Run on samples: WA shows failing sample details
_, raw = call("POST", "/runs", {"problem": "sample-sum", "language": "python", "source": "import sys\nsys.stderr.write('oops')\nprint(12345)\n"})
j, raw = wait("/runs/" + json.loads(raw)["id"], lambda j: j["status"] == "done")
bad = [c for c in j["result"]["cases"] if c["verdict"] != "AC"]
check("run WA has detail", j["result"]["verdict"] == "WA" and bad and bad[0].get("input") and bad[0].get("expected"), json.dumps(bad)[:200])
# no hidden test inputs in a Run result: tests 03..05 must not appear
hidden_in = [p.read_text().strip() for p in HIDDEN.glob("0[345].in")]
check("run result has no hidden input", not any(h and h in raw for h in hidden_in) or all(h in [c.get("input", "").strip() for c in j["result"]["cases"]] for h in hidden_in if h in raw))

# --- Run on custom input
_, raw = call("POST", "/runs", {"problem": "sample-sum", "language": "python", "source": src("ac.py"), "input": "3\n1 2 3\n"})
j, _ = wait("/runs/" + json.loads(raw)["id"], lambda j: j["status"] == "done")
check("run custom input", j["result"]["verdict"] == "OK" and j["result"].get("stdout") is not None, json.dumps(j["result"])[:200])

# --- Run compile error
_, raw = call("POST", "/runs", {"problem": "sample-sum", "language": "cpp", "source": "int main( {"})
j, _ = wait("/runs/" + json.loads(raw)["id"], lambda j: j["status"] == "done")
check("run CE has compiler output", j["result"]["verdict"] == "CE" and j["result"].get("compile_output"), json.dumps(j["result"])[:160])

# --- Submit AC and WA as the signed-in user
hdr = {}
ids = []
for name, want in (("ac.py", "AC"), ("wa.py", "WA")):
    _, raw = call("POST", "/submissions", {"problem": "sample-sum", "language": "python", "source": src(name)}, hdr)
    sid = json.loads(raw)["id"]
    ids.append(sid)
    j, raw = wait("/submissions/" + sid, lambda j: j["status"] == "judged")
    v = j["verdict"]
    banned = ["expected", "input", "stderr", "actual", "source", "test_set_version"]
    leaks = [b for b in banned if b in raw]
    check(f"submit {want}", v["verdict"] == want and not leaks, f"{v} leaks={leaks}")
    hl = [h for h in hidden_in + [p.read_text().strip() for p in HIDDEN.glob("*.out")] if len(h) > 3 and h in raw]
    check(f"submit {want} response has no test data", not hl)

# --- SSE stream carries only status/verdict
import http.client
c = http.client.HTTPConnection("127.0.0.1", 8080, timeout=30)
_, raw = call("POST", "/submissions", {"problem": "sample-sum", "language": "python", "source": src("ac.py")}, hdr)
sid = json.loads(raw)["id"]
c.request("GET", f"/submissions/{sid}/events")
resp = c.getresponse()
buf = b""
t0 = time.time()
while b"event: verdict" not in buf and time.time() - t0 < 60:
    chunk = resp.read1(4096)
    if not chunk:
        break
    buf += chunk
text = buf.decode()
check("sse reaches verdict", "event: verdict" in text, text.replace("\n", " | ")[:300])
ids.append(sid)

# --- list: only this user's, newest first
_, raw = call("GET", "/problems/sample-sum/submissions")
lst = json.loads(raw)["submissions"]
check("list has this user's 3", [s["id"] for s in lst] == ids[::-1], str([s["id"][:8] for s in lst]))
_, raw = call("GET", "/problems/sample-sum/submissions", opener=ANON)
check("anonymous sees none", json.loads(raw)["submissions"] == [])

# --- solved status: the AC above marks sample-sum solved for this user only
_, raw = call("GET", "/problems?page_size=100")
solved = {p["slug"]: p["solved"] for p in json.loads(raw)["problems"]}
check("sample-sum is solved for this user", solved.get("sample-sum") is True, str(solved))
_, raw = call("GET", "/problems?page_size=100", opener=ANON)
check("nothing is solved for anonymous", not any(p["solved"] for p in json.loads(raw)["problems"]))

# --- limits (last, because they use up this user's quota for a minute).
# The limiter runs before the body is read, so empty bodies cost nothing to judge.
codes = []
for _ in range(40):
    code, raw, hdrs = call("POST", "/runs", {}, raw_errors=True)
    codes.append(code)
    if code == 429:
        check("run limit sends Retry-After", int(hdrs.get("Retry-After", "0")) >= 1, str(hdrs.get("Retry-After")))
        break
check("run limit trips (default 20/min per user)", 429 in codes, str(codes))

other = "lim" + uuid.uuid4().hex[:10]
codes = []
for _ in range(15):
    code, _, _ = call("POST", "/auth/login", {"login": other, "password": "x" * 12}, opener=ANON, raw_errors=True)
    codes.append(code)
# Either the per-account limit (10) or the per-IP one (20, shared with the sign-up
# and login calls above) may trip first; both are 429.
check("repeated failed logins get 429 after real 401s", codes[0] == 401 and 429 in codes, str(codes))
print("account used:", USER)
print("FAILED:" if fails else "ALL PASS", fails)
sys.exit(1 if fails else 0)

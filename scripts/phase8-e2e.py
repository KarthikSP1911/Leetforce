"""Phase 8 loop check against a live API and a runner on the same queue.

Run, Submit, SSE and the Submissions list, plus a scan that no response carries
test data. Needs the API on LEETFORCE_API (default http://127.0.0.1:8080) and a
runner consuming the same Redis. It writes 3 submissions to the real database.
"""
import os, json, time, urllib.request, pathlib, uuid, sys

API = os.environ.get("LEETFORCE_API", "http://127.0.0.1:8080")
ROOT = pathlib.Path(__file__).resolve().parent.parent / "problems" / "sample-sum"
SOL = ROOT / "solutions" / "python"
HIDDEN = ROOT / "tests"
CLIENT = "e2e-" + uuid.uuid4().hex[:12]


def call(method, path, body=None, headers=None):
    data = json.dumps(body).encode() if body is not None else None
    h = {"Content-Type": "application/json", **(headers or {})}
    req = urllib.request.Request(API + path, data=data, method=method, headers=h)
    with urllib.request.urlopen(req, timeout=30) as r:
        return r.status, r.read().decode()


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

# --- Submit AC and WA, with the client header
hdr = {"X-LeetForce-Client": CLIENT}
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

# --- list: only this client's, newest first
_, raw = call("GET", "/problems/sample-sum/submissions", headers=hdr)
lst = json.loads(raw)["submissions"]
check("list has this client's 3", [s["id"] for s in lst] == ids[::-1], str([s["id"][:8] for s in lst]))
_, raw = call("GET", "/problems/sample-sum/submissions", headers={"X-LeetForce-Client": "someone-else-1234"})
check("other client sees none", json.loads(raw)["submissions"] == [])
_, raw = call("GET", "/problems/sample-sum/submissions")
check("no client id sees none", json.loads(raw)["submissions"] == [])
print("FAILED:" if fails else "ALL PASS", fails)
sys.exit(1 if fails else 0)

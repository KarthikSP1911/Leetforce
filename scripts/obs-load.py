"""Submission load for the Phase 11 dashboards and exit test.

Signs up one throwaway account, then submits sample-sum solutions (a weighted
mix of verdicts and languages) to a live API for --seconds at --rate per second,
then waits for their verdicts. Prints "account used: <name>" (the caller deletes
the account and its submissions) and a JSON summary on the last line.

  python3 scripts/obs-load.py --api http://127.0.0.1:8080 --seconds 120 --rate 0.5
"""
import argparse, http.cookiejar, json, pathlib, random, time, urllib.error, urllib.request, uuid

ROOT = pathlib.Path(__file__).resolve().parent.parent / "problems" / "sample-sum" / "solutions"
MIX = [  # (weight, language, file, extension)
    (40, "python", "ac", "py"), (15, "cpp", "ac", "cpp"), (8, "go", "ac", "go"),
    (12, "python", "wa", "py"), (6, "cpp", "wa", "cpp"), (6, "python", "re", "py"),
    (5, "python", "ce", "py"), (4, "python", "tle", "py"), (4, "cpp", "mle", "cpp"),
]

ap = argparse.ArgumentParser()
ap.add_argument("--api", default="http://127.0.0.1:8080")
ap.add_argument("--seconds", type=float, default=60)
ap.add_argument("--rate", type=float, default=0.5, help="submissions per second")
ap.add_argument("--count", type=int, default=0, help="submit exactly this many, ignoring --seconds")
ap.add_argument("--nowait", action="store_true", help="do not wait for verdicts")
ap.add_argument("--seed", type=int, default=7)
a = ap.parse_args()
random.seed(a.seed)

jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))


def call(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(a.api + path, data=data, method=method, headers={"Content-Type": "application/json"})
    with opener.open(req, timeout=30) as r:
        return json.loads(r.read().decode() or "null")


user = "obs" + uuid.uuid4().hex[:10]
print("account used:", user, flush=True)
call("POST", "/auth/signup", {"email": user + "@example.com", "username": user, "password": "obs-password-" + uuid.uuid4().hex[:6]})

ids, errors = [], 0
start = time.time()
n = 0
while (n < a.count) if a.count else (time.time() - start < a.seconds):
    _, lang, name, ext = random.choices(MIX, weights=[m[0] for m in MIX])[0]
    src = (ROOT / lang / f"{name}.{ext}").read_text()
    try:
        ids.append(call("POST", "/submissions", {"problem": "sample-sum", "language": lang, "source": src})["id"])
    except urllib.error.HTTPError as e:
        errors += 1
        print("submit failed:", e.code, flush=True)
    n += 1
    time.sleep(random.expovariate(a.rate) if a.rate > 0 else 0)

verdicts = {}
if not a.nowait:
    pending = set(ids)
    deadline = time.time() + 180
    while pending and time.time() < deadline:
        for i in list(pending):
            s = call("GET", f"/submissions/{i}")
            if s.get("verdict"):
                verdicts[s["verdict"]["verdict"]] = verdicts.get(s["verdict"]["verdict"], 0) + 1
                pending.discard(i)
        time.sleep(1)
print(json.dumps({"submitted": len(ids), "errors": errors, "verdicts": verdicts}), flush=True)

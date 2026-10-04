#!/usr/bin/env python3
"""Phase 14 driver: runs a mock contest through the API.

Called by scripts/run-mock-contest.sh after scripts/seed-mock-contest.sh. Needs
LEETFORCE_API, DATABASE_URL, bin/contestscore, and a runner judging jobs.

Users: mock1 solves A (right away) and B (after one WA); mock2 solves A and
sends one CE on B; mock3 is signed in but never registers.
Expected standings: mock1 2 solved, penalty 20 + minutes; mock2 1 solved;
mock3 absent. Also checks visibility (404 before registering, public after the
contest ends) and the 409s.
"""
import http.cookiejar
import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.error
import urllib.request

API = os.environ.get("LEETFORCE_API", "http://127.0.0.1:8080")
PASSWORD = os.environ.get("MOCK_PASSWORD", "mock-contest-pw-1")
SOL = pathlib.Path(__file__).resolve().parent.parent / "problems"
SLUG = "mock-contest"
A, B = "fizz-count", "reverse-words"
fails = []


def check(name, ok, extra=""):
    print(("PASS " if ok else "FAIL ") + name, extra)
    if not ok:
        fails.append(name)


class Client:
    def __init__(self):
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def call(self, method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(API + path, data=data, method=method, headers={"Content-Type": "application/json"})
        try:
            with self.opener.open(req, timeout=30) as r:
                return r.status, json.loads(r.read().decode() or "null")
        except urllib.error.HTTPError as e:
            raw = e.read().decode()
            try:
                return e.code, json.loads(raw)
            except ValueError:
                return e.code, raw

    def login(self, user):
        code, _ = self.call("POST", "/auth/login", {"login": user, "password": PASSWORD})
        check("login " + user, code == 200, str(code))

    def submit(self, problem, language, source, contest=True):
        body = {"problem": problem, "language": language, "source": source}
        if contest:
            body["contest_id"] = SLUG
        return self.call("POST", "/submissions", body)

    def verdict(self, sid):
        for _ in range(240):
            _, j = self.call("GET", "/submissions/" + sid)
            if j["status"] == "judged":
                return j["verdict"]["verdict"]
            time.sleep(0.5)
        raise SystemExit("timeout judging " + sid)


def ac(problem):
    return (SOL / problem / "solutions" / "python" / "ac.py").read_text()


WA_PY = "print(0)\n"
CE_CPP = "int main( {\n"

anon, m1, m2, m3 = Client(), Client(), Client(), Client()
m1.login("mock1")
m2.login("mock2")
m3.login("mock3")

# --- listing and detail
code, j = anon.call("GET", "/contests")
check("contest list has the mock contest", code == 200 and any(c["slug"] == SLUG for c in j["contests"]), str(code))
code, j = anon.call("GET", "/contests/" + SLUG)
check("contest detail: running, 3 problems", code == 200 and j["status"] == "running" and j["problem_count"] == 3, str(j))
check("unknown contest is 404", anon.call("GET", "/contests/nope")[0] == 404)

# --- visibility before registering
code, j = anon.call("GET", "/problems?page_size=100")
check("contest problems hidden from the public list", code == 200 and not {A, B} & {p["slug"] for p in j["problems"]})
check("contest problem 404 for anonymous", anon.call("GET", "/problems/" + A)[0] == 404)
check("contest problem 404 for unregistered user", m3.call("GET", "/problems/" + A)[0] == 404)
check("contest problem list 404 for unregistered user", m3.call("GET", f"/contests/{SLUG}/problems")[0] == 404)
code, j = m3.submit(A, "python", ac(A))
check("submit by unregistered user is 409", code == 409, str(code))
check("register needs sign-in", anon.call("POST", f"/contests/{SLUG}/register")[0] == 401)

# --- register
for name, c in (("mock1", m1), ("mock2", m2)):
    code, j = c.call("POST", f"/contests/{SLUG}/register")
    check("register " + name, code == 200 and j["registered"] is True, str(code))
check("register is idempotent", m1.call("POST", f"/contests/{SLUG}/register")[0] == 200)
check("registered user sees the problem", m1.call("GET", "/problems/" + A)[0] == 200)
code, j = m1.call("GET", f"/contests/{SLUG}/problems")
check("contest problems A, B, C", code == 200 and [p["label"] for p in j["problems"]] == ["A", "B", "C"], str(j))
code, _ = m1.submit("sample-sum", "python", "print(0)")
check("problem outside the contest is 422", code == 422, str(code))

# --- submissions: mock1 A=AC; B=WA then AC. mock2 A=AC; B=CE.
verdicts = {}


def go(client, name, problem, lang, src, want):
    code, j = client.submit(problem, lang, src)
    if code != 202:
        check(f"{name} submit {problem} accepted", False, f"{code} {j}")
        return
    got = client.verdict(j["id"])
    verdicts[(name, problem, want)] = got
    check(f"{name} {problem} verdict {want}", got == want, got)
    time.sleep(1.1)  # keep submit order and timestamps distinct


go(m1, "mock1", A, "python", ac(A), "AC")
go(m1, "mock1", B, "python", WA_PY, "WA")
go(m1, "mock1", B, "python", ac(B), "AC")
go(m1, "mock1", B, "python", WA_PY, "WA")  # after the first AC: must not count
go(m2, "mock2", A, "python", ac(A), "AC")
go(m2, "mock2", B, "cpp", CE_CPP, "CE")

# --- scores, from contest.Score over the real verdicts
out = subprocess.run(["bin/contestscore", SLUG], capture_output=True, text=True, check=True).stdout
standings = json.loads(out)
by_user = {s["user_id"]: s for s in standings}
check("two users ranked (mock3 absent)", len(standings) == 2, out[:200])
top, second = standings[0], standings[1]
check("rank 1: 2 solved, penalty 20 + minutes", top["rank"] == 1 and top["solved"] == 2 and 20 <= top["penalty_minutes"] < 30, str(top))
check("rank 2: 1 solved, CE not counted", second["rank"] == 2 and second["solved"] == 1 and second["penalty_minutes"] < 10, str(second))
b = [p for p in top["per_problem"] if p["problem"] == B]
check("B: one rejected attempt before the AC, later WA ignored", len(b) == 1 and b[0]["rejected"] == 1 and b[0]["solved"], str(b))

# --- after the end: problems public, submissions closed
subprocess.run(["psql", os.environ["DATABASE_URL"], "-qAt", "-c",
                f"UPDATE contests SET starts_at = now() - interval '2 hours', ends_at = now() - interval '1 minute' WHERE slug = '{SLUG}'"],
               check=True)
code, j = anon.call("GET", f"/contests/{SLUG}/problems")
check("ended contest problems are public", code == 200 and len(j["problems"]) == 3, str(code))
check("ended contest problem is public", anon.call("GET", "/problems/" + A)[0] == 200)
code, _ = m1.submit(A, "python", ac(A))
check("submit after the end is 409", code == 409, str(code))
check("register after the end is 409", m3.call("POST", f"/contests/{SLUG}/register")[0] == 409)

print("FAILED: " + ", ".join(fails) if fails else "all mock contest checks passed")
sys.exit(1 if fails else 0)

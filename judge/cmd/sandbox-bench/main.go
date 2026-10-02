//go:build linux

// Command sandbox-bench compares the nsjail and gVisor sandbox backends (Phase 6,
// ADR 0013). It runs the same workloads under both, interleaved so that drift on
// a shared host hits both equally, and prints a markdown table of median and p95.
// It needs root, nsjail, runsc and gcc (run it through `make bench-sandbox`).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/sandbox"
)

const (
	helloC = `#include <stdio.h>
int main(void) { puts("hello"); return 0; }`

	// About half a second of integer arithmetic natively.
	cpuC = `#include <stdio.h>
int main(void) {
	unsigned long x = 1;
	for (unsigned long i = 0; i < 600000000UL; i++) x = x * 6364136223846793005UL + i;
	printf("%lu\n", x);
	return 0;
}`

	// Many cheap system calls: the worst case for a user-space kernel.
	sysC = `#define _GNU_SOURCE
#include <fcntl.h>
#include <stdio.h>
#include <sys/syscall.h>
#include <unistd.h>
int main(void) {
	int z = open("/dev/zero", O_RDONLY), n = open("/dev/null", O_WRONLY);
	char c = 0;
	long s = 0;
	for (int i = 0; i < 100000; i++) s += syscall(SYS_getppid);
	for (int i = 0; i < 50000; i++) { s += read(z, &c, 1); s += write(n, &c, 1); }
	printf("%ld\n", s);
	return 0;
}`

	// Touches 64 MiB, so peak memory minus 64 MiB is the sandbox overhead.
	memC = `#include <stdlib.h>
#include <stdio.h>
int main(void) {
	size_t n = 64UL << 20;
	volatile char *p = malloc(n);
	for (size_t i = 0; i < n; i += 4096) p[i] = 1;
	printf("%d\n", p[n / 2]);
	return 0;
}`
	memBytes = 64 << 20
)

type sample struct {
	wall time.Duration // the whole sandbox.Run call, host-measured
	cpu  time.Duration
	mem  uint64
	pids uint64
}

type workload struct {
	name string
	run  func(ctx context.Context, b sandbox.Backend) (sample, error)
}

func main() {
	reps := flag.Int("reps", 15, "repetitions for the micro workloads")
	compileReps := flag.Int("compile-reps", 3, "repetitions for the compile+run workloads")
	problemDir := flag.String("problem", "problems/sample-sum", "problem for the compile+run workloads")
	dropCaches := flag.Bool("drop-caches", true, "drop the page cache before the cold-start sample (needs root)")
	skipLangs := flag.Bool("skip-langs", false, "skip the Go/Java/C++ compile+run workloads")
	flag.Parse()

	if os.Geteuid() != 0 {
		fatalf("needs root: run through `make bench-sandbox`")
	}
	ctx := context.Background()
	backends := []sandbox.Backend{sandbox.BackendNsjail, sandbox.BackendGVisor}

	dir, err := os.MkdirTemp("/var/tmp", "lf-bench-")
	must(err)
	defer func() { _ = os.RemoveAll(dir) }()
	must(os.Chmod(dir, 0o755)) //nolint:gosec // traversable by the sandbox user
	bins := map[string]string{}
	for name, src := range map[string]string{"hello": helloC, "cpu": cpuC, "sys": sysC, "mem": memC} {
		bins[name] = buildC(ctx, dir, name, src)
	}

	run := func(name string) func(context.Context, sandbox.Backend) (sample, error) {
		return func(ctx context.Context, b sandbox.Backend) (sample, error) {
			return runBin(ctx, b, dir, bins[name])
		}
	}
	micro := []workload{
		{"hello world run", run("hello")},
		{"CPU-bound (600M mult-add)", run("cpu")},
		{"syscall-heavy (200k syscalls)", run("sys")},
		{"memory (touch 64 MiB)", run("mem")},
	}

	fmt.Printf("# Sandbox benchmark: nsjail vs gVisor\n\n")
	fmt.Printf("host: %s; %d micro reps, %d compile reps; values are median / p95\n\n", hostInfo(), *reps, *compileReps)

	// Cold start: the first run of each backend, caches dropped first.
	cold := map[sandbox.Backend]sample{}
	for _, b := range backends {
		if *dropCaches {
			dropPageCache()
		}
		s, err := run("hello")(ctx, b)
		must(err)
		cold[b] = s
	}

	results := map[string]map[sandbox.Backend][]sample{}
	for _, w := range micro {
		results[w.name] = map[sandbox.Backend][]sample{}
		// One unmeasured warm-up per backend, then interleaved repetitions.
		for _, b := range backends {
			_, err := w.run(ctx, b)
			must(err)
		}
		for i := 0; i < *reps; i++ {
			for _, b := range backends {
				s, err := w.run(ctx, b)
				must(err)
				results[w.name][b] = append(results[w.name][b], s)
			}
		}
	}

	fmt.Println("| Workload | nsjail wall | gVisor wall | ratio | nsjail CPU | gVisor CPU | nsjail peak mem | gVisor peak mem | nsjail peak pids | gVisor peak pids |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|")
	fmt.Printf("| cold start (first run, caches dropped=%v) | %s | %s | %.1fx | %s | %s | %s | %s | %d | %d |\n",
		*dropCaches, ms(cold[backends[0]].wall), ms(cold[backends[1]].wall),
		ratio(cold[backends[1]].wall, cold[backends[0]].wall),
		ms(cold[backends[0]].cpu), ms(cold[backends[1]].cpu),
		mib(cold[backends[0]].mem), mib(cold[backends[1]].mem), cold[backends[0]].pids, cold[backends[1]].pids)
	for _, w := range micro {
		n, g := results[w.name][backends[0]], results[w.name][backends[1]]
		fmt.Printf("| %s | %s | %s | %.1fx | %s | %s | %s | %s | %d | %d |\n", w.name,
			stat(n, func(s sample) time.Duration { return s.wall }), stat(g, func(s sample) time.Duration { return s.wall }),
			ratio(median(g, func(s sample) time.Duration { return s.wall }), median(n, func(s sample) time.Duration { return s.wall })),
			stat(n, func(s sample) time.Duration { return s.cpu }), stat(g, func(s sample) time.Duration { return s.cpu }),
			memStat(n), memStat(g), medianU(n, func(s sample) uint64 { return s.pids }), medianU(g, func(s sample) uint64 { return s.pids }))
	}
	nm := medianU(results["memory (touch 64 MiB)"][backends[0]], func(s sample) uint64 { return s.mem })
	gm := medianU(results["memory (touch 64 MiB)"][backends[1]], func(s sample) uint64 { return s.mem })
	nh := medianU(results["hello world run"][backends[0]], func(s sample) uint64 { return s.mem })
	gh := medianU(results["hello world run"][backends[1]], func(s sample) uint64 { return s.mem })
	fmt.Printf("\nPeak memory overhead (hello world): nsjail %s, gVisor %s. Touching 64 MiB: nsjail %s over, gVisor %s over.\n",
		mibU(nh), mibU(gh), mibU(sub(nm, memBytes)), mibU(sub(gm, memBytes)))

	if *skipLangs {
		return
	}
	p, err := problem.Load(*problemDir)
	must(err)
	fmt.Printf("\n## Compile + run (engine.Judge of %s, ac solution, %d tests)\n\n", p.Spec.Slug, len(p.Tests))
	fmt.Println("| Language | nsjail total | gVisor total | ratio | nsjail max case CPU | gVisor max case CPU | nsjail max case mem | gVisor max case mem |")
	fmt.Println("|---|---|---|---|---|---|---|---|")
	for _, lang := range []string{"cpp", "go", "java"} {
		src, err := os.ReadFile(filepath.Join(*problemDir, "solutions", lang, "ac."+ext(lang))) //nolint:gosec // developer-supplied path
		must(err)
		// One unmeasured judge per backend first, so toolchain files are in the page
		// cache for both and the first backend does not pay for it.
		for _, b := range backends {
			_, err := judgeOnce(ctx, b, p, lang, src)
			must(err)
		}
		per := map[sandbox.Backend][]langSample{}
		for i := 0; i < *compileReps; i++ {
			for _, b := range backends {
				s, err := judgeOnce(ctx, b, p, lang, src)
				must(err)
				per[b] = append(per[b], s)
			}
		}
		n, g := per[backends[0]], per[backends[1]]
		nt := medianD(n, func(s langSample) time.Duration { return s.total })
		gt := medianD(g, func(s langSample) time.Duration { return s.total })
		fmt.Printf("| %s | %s | %s | %.1fx | %s | %s | %s | %s |\n", lang,
			statD(n, func(s langSample) time.Duration { return s.total }), statD(g, func(s langSample) time.Duration { return s.total }),
			ratio(gt, nt),
			ms(medianD(n, func(s langSample) time.Duration { return s.cpu })), ms(medianD(g, func(s langSample) time.Duration { return s.cpu })),
			mibU(medianUD(n)), mibU(medianUD(g)))
	}
}

type langSample struct {
	total time.Duration
	cpu   time.Duration // slowest test's CPU time
	mem   uint64        // largest test's peak memory
}

func judgeOnce(ctx context.Context, b sandbox.Backend, p *problem.Problem, lang string, src []byte) (langSample, error) {
	if err := os.Setenv(sandbox.BackendEnv, string(b)); err != nil {
		return langSample{}, err
	}
	start := time.Now()
	rep, err := (&engine.Engine{}).Judge(ctx, p, lang, src, engine.Options{ContinueOnFail: true})
	if err != nil {
		return langSample{}, fmt.Errorf("%s under %s: %w", lang, b, err)
	}
	total := time.Since(start)
	if rep.Overall.Verdict != "AC" {
		return langSample{}, fmt.Errorf("%s under %s: overall %v, not AC (compile output: %q)", lang, b, rep.Overall.Verdict, rep.CompileOutput)
	}
	s := langSample{total: total}
	for _, c := range rep.Cases {
		s.cpu = max(s.cpu, c.Time)
		s.mem = max(s.mem, c.Memory)
	}
	return s, nil
}

func runBin(ctx context.Context, b sandbox.Backend, dir, bin string) (sample, error) {
	spec := sandbox.Spec{
		Argv:          []string{bin},
		ReadOnlyBinds: []string{dir},
		Limits:        sandbox.DefaultLimits(),
		Backend:       b,
	}
	spec.Limits.WallTime = 60 * time.Second
	spec.Limits.CPUTime = 60 * time.Second
	start := time.Now()
	res, err := sandbox.Run(ctx, spec)
	wall := time.Since(start)
	if err != nil {
		return sample{}, fmt.Errorf("%s under %s: %w", filepath.Base(bin), b, err)
	}
	if res.ExitCode != 0 || res.Signal != 0 {
		return sample{}, fmt.Errorf("%s under %s: exit=%d signal=%v stderr=%q", filepath.Base(bin), b, res.ExitCode, res.Signal, res.Stderr)
	}
	return sample{wall: wall, cpu: res.RawCPUTime, mem: res.RawPeakMemoryBytes, pids: res.RawPeakPIDs}, nil
}

func buildC(ctx context.Context, dir, name, src string) string {
	srcPath := filepath.Join(dir, name+".c")
	must(os.WriteFile(srcPath, []byte(src), 0o644)) //nolint:gosec // read by gcc only
	bin := filepath.Join(dir, name)
	out, err := exec.CommandContext(ctx, "gcc", "-O2", "-static", "-o", bin, srcPath).CombinedOutput() //nolint:gosec // fixed inputs
	if err != nil {
		fatalf("gcc %s: %v\n%s", name, err, out)
	}
	return bin
}

func dropPageCache() {
	_ = exec.CommandContext(context.Background(), "sync").Run()
	_ = os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0o200)
}

func hostInfo() string {
	var parts []string
	if b, err := exec.CommandContext(context.Background(), "uname", "-sr").Output(); err == nil {
		parts = append(parts, strings.TrimSpace(string(b)))
	}
	if b, err := exec.CommandContext(context.Background(), "runsc", "--version").Output(); err == nil {
		parts = append(parts, strings.Fields(string(b))[0]+" "+strings.Fields(string(b))[2])
	}
	if b, err := exec.CommandContext(context.Background(), "nproc").Output(); err == nil {
		parts = append(parts, strings.TrimSpace(string(b))+" vCPU")
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts = append(parts, "load at start "+strings.Fields(string(b))[0])
	}
	return strings.Join(parts, ", ")
}

func ext(lang string) string {
	return map[string]string{"cpp": "cpp", "go": "go", "java": "java"}[lang]
}

// Statistics. Percentiles use the nearest-rank method.

func pct[T any](xs []T, less func(a, b T) bool, p float64) T {
	s := append([]T(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return less(s[i], s[j]) })
	idx := int(float64(len(s))*p+0.999999) - 1
	idx = min(max(idx, 0), len(s)-1)
	return s[idx]
}

func durs(xs []sample, f func(sample) time.Duration) []time.Duration {
	out := make([]time.Duration, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func lessD(a, b time.Duration) bool { return a < b }

func median(xs []sample, f func(sample) time.Duration) time.Duration {
	return pct(durs(xs, f), lessD, 0.5)
}

func stat(xs []sample, f func(sample) time.Duration) string {
	d := durs(xs, f)
	return ms(pct(d, lessD, 0.5)) + " / " + ms(pct(d, lessD, 0.95))
}

func medianU(xs []sample, f func(sample) uint64) uint64 {
	u := make([]uint64, len(xs))
	for i, x := range xs {
		u[i] = f(x)
	}
	return pct(u, func(a, b uint64) bool { return a < b }, 0.5)
}

func memStat(xs []sample) string {
	u := make([]uint64, len(xs))
	for i, x := range xs {
		u[i] = x.mem
	}
	less := func(a, b uint64) bool { return a < b }
	return mibU(pct(u, less, 0.5)) + " / " + mibU(pct(u, less, 0.95))
}

func medianD(xs []langSample, f func(langSample) time.Duration) time.Duration {
	d := make([]time.Duration, len(xs))
	for i, x := range xs {
		d[i] = f(x)
	}
	return pct(d, lessD, 0.5)
}

func statD(xs []langSample, f func(langSample) time.Duration) string {
	d := make([]time.Duration, len(xs))
	for i, x := range xs {
		d[i] = f(x)
	}
	return secs(pct(d, lessD, 0.5)) + " / " + secs(pct(d, lessD, 0.95))
}

func medianUD(xs []langSample) uint64 {
	u := make([]uint64, len(xs))
	for i, x := range xs {
		u[i] = x.mem
	}
	return pct(u, func(a, b uint64) bool { return a < b }, 0.5)
}

func sub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// ratio is gVisor over nsjail.
func ratio(g, n time.Duration) float64 {
	if n == 0 {
		return 0
	}
	return float64(g) / float64(n)
}

func ms(d time.Duration) string   { return fmt.Sprintf("%.0f ms", float64(d)/float64(time.Millisecond)) }
func secs(d time.Duration) string { return fmt.Sprintf("%.1f s", d.Seconds()) }
func mib(b uint64) string         { return mibU(b) }
func mibU(b uint64) string        { return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20)) }

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sandbox-bench: "+format+"\n", args...)
	os.Exit(1)
}

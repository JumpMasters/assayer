package run_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JumpMasters/assayer/internal/adapter/claudecode/run"
	"github.com/JumpMasters/assayer/internal/adapter/conformance"
	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// The fixtures under testdata are verbatim output from the real harness,
// recorded once against version 2.1.220 and kept whole rather than trimmed to
// the fields this adapter reads. A fixture reduced to what the parser wants
// cannot show the parser reading the wrong thing, and cannot show the harness
// changing shape around it.
const (
	completed = "success.json" // a run that finished the work
	turnCap   = "turns.json"   // stopped at --max-turns 1
	budgetCap = "budget.json"  // stopped at --max-budget-usd 0.001
)

// stub writes an executable standing in for the harness. The adapter drives a
// subprocess, so the seam under test is a subprocess; anything that replaced it
// with an injected interface would be testing the injection.
func stub(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatalf("writing the stub harness: %v", err)
	}
	return path
}

// replays is a stub that prints a recorded result and exits with code.
func replays(t *testing.T, fixture string, code int) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("locating fixture %s: %v", fixture, err)
	}
	return stub(t, "cat '"+abs+"'\nexit "+strconv.Itoa(code))
}

func work(t *testing.T) *assay.Assignment {
	t.Helper()
	return &assay.Assignment{
		Instruction: "make the failing test pass",
		Dir:         t.TempDir(),
	}
}

func TestSitReadsARunThatFinished(t *testing.T) {
	a := run.Adapter{Bin: replays(t, completed, 0)}

	s, err := a.Sit(context.Background(), work(t))
	if err != nil {
		t.Fatalf("Sit: %v", err)
	}

	if s.Stop != assay.StopComplete {
		t.Errorf("Stop = %v, want complete", s.Stop)
	}
	if !s.Stop.Decides() {
		t.Error("a completed run does not permit a verdict; nothing else ever will")
	}
	if s.Native != "4006eaf7-f396-4b7b-9c0d-b2512fc1baf8" {
		t.Errorf("Native = %q, want the session identifier from the result", s.Native)
	}
	if s.Adapter != a.ID() || s.Tier != a.Tier() || s.Guarantees != a.Guarantees() {
		t.Error("the sitting does not carry the adapter that produced it")
	}
	if s.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", s.ExitCode)
	}
	if s.Wall <= 0 {
		t.Error("Wall is not set; how long the run took is measured here, not reported")
	}
	if s.Usage.Wall != 0 {
		t.Error("Usage.Wall is set; it means the span between a transcript's " +
			"first and last timestamps, which is not what a sitting measures, and " +
			"carrying both on one field invites a comparison between them")
	}
	if s.Turns != 1 {
		t.Errorf("Turns = %d, want 1 as the fixture reports", s.Turns)
	}
}

// TestSitCountsEveryInputTokenAndTheMoney pins the arithmetic against the
// recorded run. The input figure is the sum of the fresh, written-to-cache and
// read-from-cache fields: on the store this project measured, cached input was
// 99.97% of the total, so reading the first field alone reports a run three
// orders of magnitude cheaper than it was.
func TestSitCountsEveryInputTokenAndTheMoney(t *testing.T) {
	a := run.Adapter{Bin: replays(t, completed, 0)}

	s, err := a.Sit(context.Background(), work(t))
	if err != nil {
		t.Fatalf("Sit: %v", err)
	}

	const wantInput = 2 + 1637 + 0
	if s.Usage.InputTokens != wantInput {
		t.Errorf("InputTokens = %d, want %d (fresh + written to cache + read from cache)",
			s.Usage.InputTokens, wantInput)
	}
	if s.Usage.OutputTokens != 4 {
		t.Errorf("OutputTokens = %d, want 4", s.Usage.OutputTokens)
	}

	// The recorded completed run happens to have read nothing from cache, so on
	// that fixture alone the sum is indistinguishable from one that ignores the
	// field entirely — and by measurement that field carries 96.5% of input
	// across the store. The capped run read 1455 tokens from cache and is the
	// fixture that makes the claim testable.
	capped := run.Adapter{Bin: replays(t, turnCap, 1)}
	c, err := capped.Sit(context.Background(), work(t))
	if err != nil {
		t.Fatalf("Sit: %v", err)
	}
	const wantCappedInput = 2 + 197 + 1455
	if c.Usage.InputTokens != wantCappedInput {
		t.Errorf("InputTokens = %d, want %d; tokens read from cache are being "+
			"dropped, which is most of what a run is billed for",
			c.Usage.InputTokens, wantCappedInput)
	}
	// $0.010922250000000001 in the fixture. Money is carried as integer
	// millionths precisely so that this comparison is exact.
	if s.Usage.CostMicroUSD != 10922 {
		t.Errorf("CostMicroUSD = %d, want 10922", s.Usage.CostMicroUSD)
	}
}

// TestSitReadsCapsFromTheHarnessOwnAccount is the ERROR-not-FAIL boundary at the
// point it is easiest to get wrong. Both measured cap hits exited 1 — the same
// status a harness returns when it has genuinely fallen over — so the exit code
// cannot be the field that decides, and the subtype has to be.
//
// The stub is told to exit 1 because that is what the recorded runs did; the
// status is not in the fixtures, which are stdout, so it is written down in
// testdata/README.md instead. This test therefore proves the adapter reads the
// subtype rather than the status, and does not corroborate the measurement.
func TestSitReadsCapsFromTheHarnessOwnAccount(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    assay.StopReason
	}{
		{"turn cap", turnCap, assay.StopTurns},
		{"budget cap", budgetCap, assay.StopBudget},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := run.Adapter{Bin: replays(t, tt.fixture, 1)}

			s, err := a.Sit(context.Background(), work(t))
			if err != nil {
				t.Fatalf("Sit: %v", err)
			}
			if s.Stop != tt.want {
				t.Errorf("Stop = %v, want %v", s.Stop, tt.want)
			}
			if s.ExitCode != 1 {
				t.Errorf("ExitCode = %d, want the status the stub was given", s.ExitCode)
			}
			if s.Stop.Decides() {
				t.Error("a capped sitting permits a verdict; the work never finished, " +
					"so nothing it produced is evidence about whether it can be done")
			}
		})
	}
}

// TestACappedSittingCanReportNoTokensAgainstRealMoney records something the
// harness's own schema does not imply and the recorded run shows: when a budget
// cap stops a sitting, the usage block comes back zeroed while the cost does
// not. Money was spent and the tokens that bought it are not reported.
//
// Nothing is done about it here beyond letting it be true — reconstructing the
// counts from the per-model breakdown would couple this adapter to a second
// shape of the harness's accounting for a diagnostic figure. It is pinned so
// that a consumer reading zero tokens against a real cost finds this test rather
// than a bug report.
func TestACappedSittingCanReportNoTokensAgainstRealMoney(t *testing.T) {
	a := run.Adapter{Bin: replays(t, budgetCap, 1)}

	s, err := a.Sit(context.Background(), work(t))
	if err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if s.Usage.InputTokens != 0 || s.Usage.OutputTokens != 0 {
		t.Errorf("the recorded budget-capped run reported %d/%d tokens; the "+
			"fixture has them zeroed, so either the fixture or this expectation moved",
			s.Usage.InputTokens, s.Usage.OutputTokens)
	}
	if s.Usage.CostMicroUSD != 4725 {
		t.Errorf("CostMicroUSD = %d, want 4725; a cap that stopped the run at "+
			"$0.001 still cost $0.0047", s.Usage.CostMicroUSD)
	}
}

func TestSitAsksForAHermeticRun(t *testing.T) {
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv")
	stdin := filepath.Join(dir, "stdin")
	abs, err := filepath.Abs(filepath.Join("testdata", completed))
	if err != nil {
		t.Fatal(err)
	}
	bin := stub(t, "printf '%s\\n' \"$@\" > '"+argv+"'\ncat > '"+stdin+"'\ncat '"+abs+"'")

	a := run.Adapter{Bin: bin}
	assignment := assay.Assignment{
		Instruction: "make the failing test pass",
		Dir:         dir,
		Model:       "some-model",
		Tools:       []string{"Read", "Edit"},
		Settings:    `{"permissions":{}}`,
		Caps: assay.Caps{
			BudgetMicroUSD: 2_500_000,
			Turns:          7,
			Wall:           time.Minute,
		},
	}
	if _, err := a.Sit(context.Background(), &assignment); err != nil {
		t.Fatalf("Sit: %v", err)
	}

	args := readArgs(t, argv)

	for _, want := range []struct {
		flag     string
		value    string
		hasValue bool
	}{
		{flag: "--print"},
		{flag: "--bare"},
		{flag: "--strict-mcp-config"},
		{flag: "--output-format", value: "json", hasValue: true},
		// An empty list: load no settings file the machine happens to have.
		{flag: "--setting-sources", value: "", hasValue: true},
		{flag: "--model", value: "some-model", hasValue: true},
		{flag: "--allowedTools", value: "Read,Edit", hasValue: true},
		{flag: "--settings", value: `{"permissions":{}}`, hasValue: true},
		{flag: "--max-budget-usd", value: "2.5", hasValue: true},
		{flag: "--max-turns", value: "7", hasValue: true},
	} {
		if !hasFlag(args, want.flag, want.value, want.hasValue) {
			t.Errorf("the harness was not asked for %s %q; argv was %q",
				want.flag, want.value, args)
		}
	}

	// The keystone, stated as an absence. --fallback-model is opt-in on this
	// binary, and passing it would let the harness quietly answer with a model
	// other than the one under examination — which is the single thing a
	// regression test must not permit.
	for _, arg := range args {
		if arg == "--fallback-model" {
			t.Error("the harness was given --fallback-model; the model under " +
				"examination could then change mid-run without the report saying so")
		}
	}

	// A wall cap is held by this adapter, not by the harness, so it must not
	// appear as a flag; declaring it supervised and then passing a flag would be
	// a guarantee described wrongly.
	for _, arg := range args {
		if strings.Contains(arg, "timeout") {
			t.Errorf("unexpected timeout flag %q; the wall cap is supervised", arg)
		}
	}

	got, err := os.ReadFile(stdin)
	if err != nil {
		t.Fatalf("reading what was put on the harness's stdin: %v", err)
	}
	if string(got) != assignment.Instruction {
		t.Errorf("stdin was %q, want the instruction; it goes there rather than "+
			"in the argument list because a distilled instruction has no bound "+
			"and an argument list does", got)
	}
}

// TestSitOmitsFlagsTheAssignmentDidNotAskFor is the other half of the hermeticity
// check. Sending --model "" or --settings "" on every default assignment would
// pass the positive test above and change what the harness does; print mode
// silently ignores settings that fail to validate, so it would not even be
// visible at runtime.
func TestSitOmitsFlagsTheAssignmentDidNotAskFor(t *testing.T) {
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv")
	abs, err := filepath.Abs(filepath.Join("testdata", completed))
	if err != nil {
		t.Fatal(err)
	}
	bin := stub(t, "printf '%s\n' \"$@\" > '"+argv+"'\ncat '"+abs+"'")

	assignment := work(t)
	assignment.Dir = dir
	if _, err := (run.Adapter{Bin: bin}).Sit(context.Background(), assignment); err != nil {
		t.Fatalf("Sit: %v", err)
	}

	args := readArgs(t, argv)
	for _, flag := range []string{
		"--model", "--allowedTools", "--settings", "--max-budget-usd", "--max-turns",
	} {
		for _, arg := range args {
			if arg == flag {
				t.Errorf("the harness was given %s for an assignment that did not "+
					"ask for it; argv was %q", flag, args)
			}
		}
	}
}

func TestSitRunsInTheDirectoryItWasGiven(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "cwd")
	bin := stub(t, "pwd > '"+out+"'")

	a := run.Adapter{Bin: bin}
	assignment := work(t)
	assignment.Dir = dir
	if _, err := a.Sit(context.Background(), assignment); err != nil {
		t.Fatalf("Sit: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading the harness's working directory: %v", err)
	}
	// Both sides are resolved before comparing: a temporary directory on this
	// platform is reached through a symlink, and the shell reports the logical
	// path rather than the one it resolves to.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	ran, err := filepath.EvalSymlinks(strings.TrimSpace(string(got)))
	if err != nil {
		t.Fatalf("resolving the directory the harness reported: %v", err)
	}
	if ran != want {
		t.Errorf("the harness ran in %q, want %q", ran, want)
	}
}

// TestSitHoldsTheWallCapItself is the supervised guarantee, exercised. The stub
// replaces its own shell so that the process the adapter holds is the process
// that has to die; a stub that forked would leave the adapter waiting on a pipe
// held open by a child it never knew about.
func TestSitHoldsTheWallCapItself(t *testing.T) {
	a := run.Adapter{Bin: stub(t, "exec sleep 30")}
	assignment := work(t)
	assignment.Caps.Wall = 150 * time.Millisecond

	started := time.Now()
	s, err := a.Sit(context.Background(), assignment)
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if s.Stop != assay.StopWall {
		t.Errorf("Stop = %v, want wall", s.Stop)
	}
	if s.Stop.Decides() {
		t.Error("a sitting killed by the wall cap permits a verdict")
	}
	if elapsed > 3*time.Second {
		t.Errorf("Sit took %v against a 150ms cap; the cap is not being held", elapsed)
	}
}

// TestTheWallCapSurvivesAForkingHarness is the case the previous version of this
// file wrote around rather than defended against.
//
// Killing a process is not the same as being able to stop waiting for it:
// exec.CommandContext ends the child it started, and the wait then reads the
// output pipes until end of file, which never arrives while a grandchild still
// holds them. Without cmd.WaitDelay this hangs indefinitely, and the guarantee
// the adapter declares would be one it keeps only against harnesses that do not
// fork — which the universal command-template runner, wrapping whatever a user
// names, cannot promise.
func TestTheWallCapSurvivesAForkingHarness(t *testing.T) {
	a := run.Adapter{Bin: stub(t, "sleep 30 &\nsleep 30")}
	assignment := work(t)
	assignment.Caps.Wall = 150 * time.Millisecond

	done := make(chan assay.Sitting, 1)
	go func() {
		s, err := a.Sit(context.Background(), assignment)
		if err != nil {
			t.Errorf("Sit: %v", err)
		}
		done <- s
	}()

	select {
	case s := <-done:
		if s.Stop != assay.StopWall {
			t.Errorf("Stop = %v, want wall", s.Stop)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Sit never returned against a 150ms wall cap; a descendant is " +
			"holding the output pipes and nothing closes them")
	}
}

// TestSitDoesNotHandTheHarnessTheMachinesEnvironment is the third source of
// ambient configuration, and the one no flag covers. The harness reads its model
// and its backend from the environment, so a developer with ANTHROPIC_MODEL
// exported would otherwise get sittings run against a model no report names.
func TestSitDoesNotHandTheHarnessTheMachinesEnvironment(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "some-other-model")
	t.Setenv("ANTHROPIC_BASE_URL", "https://somewhere.else.invalid")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("ANTHROPIC_API_KEY", "not-a-real-key")

	dir := t.TempDir()
	env := filepath.Join(dir, "env")
	bin := stub(t, "env > '"+env+"'")

	assignment := work(t)
	assignment.Dir = dir
	if _, err := (run.Adapter{Bin: bin}).Sit(context.Background(), assignment); err != nil {
		t.Fatalf("Sit: %v", err)
	}

	got, err := os.ReadFile(env)
	if err != nil {
		t.Fatalf("reading the harness's environment: %v", err)
	}
	for _, name := range []string{"ANTHROPIC_MODEL", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_USE_BEDROCK"} {
		if strings.Contains(string(got), name+"=") {
			t.Errorf("the harness was handed %s; it reads that variable, so the "+
				"sitting would run under a condition the report cannot name", name)
		}
	}
	// The allowlist is not a blanket refusal: authentication has to survive it,
	// or no sitting can run at all.
	if !strings.Contains(string(got), "ANTHROPIC_API_KEY=") {
		t.Error("the harness was not handed ANTHROPIC_API_KEY; under --bare that " +
			"is the only way it can authenticate")
	}
	if !strings.Contains(string(got), "PATH=") {
		t.Error("the harness was not handed PATH; the agent runs shell commands")
	}
}

// TestSitReturnsTheCallersCancellationWithoutASitting keeps the two kinds of
// stopping apart. A cap is a result about the run and produces a sitting; a
// caller changing its mind is not and must not, or a cancelled suite would fill
// the ledger with sittings nobody administered.
func TestSitReturnsTheCallersCancellationWithoutASitting(t *testing.T) {
	a := run.Adapter{Bin: stub(t, "exec sleep 30")}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	s, err := a.Sit(ctx, work(t))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Sit returned %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(s, assay.Sitting{}) {
		t.Errorf("Sit returned a sitting alongside a cancellation: %+v", s)
	}
}

func TestSitReportsAHarnessItCannotStart(t *testing.T) {
	a := run.Adapter{Bin: filepath.Join(t.TempDir(), "no-such-harness")}

	_, err := a.Sit(context.Background(), work(t))
	if !errors.Is(err, port.ErrUnavailable) {
		t.Errorf("Sit against a missing harness returned %v, want ErrUnavailable; "+
			"a harness that is not installed is not a regression", err)
	}
}

// TestSitStartsNothingForARefusedAssignment is the guarantee that a refusal is
// free. The stub records having run at all, so the assertion is about the
// process rather than about the error.
func TestSitStartsNothingForARefusedAssignment(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	a := run.Adapter{Bin: stub(t, "touch '"+marker+"'")}

	_, err := a.Sit(context.Background(), &assay.Assignment{})
	if !errors.Is(err, port.ErrRefused) {
		t.Fatalf("Sit with an empty assignment returned %v, want ErrRefused", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the harness ran for an assignment that was refused; a refusal " +
			"that costs money is not a refusal")
	}
}

// TestSitCannotClassifyOutputItCannotRead separates two ways of ending badly. A
// harness that exited non-zero at least said something went wrong; one that
// exited cleanly and wrote nonsense leaves nothing to say, and saying nothing is
// the honest answer. Both reach ERROR.
func TestSitCannotClassifyOutputItCannotRead(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   assay.StopReason
	}{
		{"clean exit, unreadable output", "echo 'not json'", assay.StopUnknown},
		{"non-zero exit, unreadable output", "echo 'not json' >&2\nexit 3", assay.StopFailed},
		{"clean exit, some other json", "echo '{\"type\":\"system\"}'", assay.StopUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := run.Adapter{Bin: stub(t, tt.script)}

			s, err := a.Sit(context.Background(), work(t))
			if err != nil {
				t.Fatalf("Sit: %v", err)
			}
			if s.Stop != tt.want {
				t.Errorf("Stop = %v, want %v", s.Stop, tt.want)
			}
			if s.Stop.Decides() {
				t.Error("output nobody could read permits a verdict")
			}
		})
	}
}

// TestSitCarriesTheHarnessOwnWords pins that stderr travels with the sitting. An
// ERROR verdict whose reason line is Assayer paraphrasing a harness it did not
// understand is worse than one quoting it.
func TestSitCarriesTheHarnessOwnWords(t *testing.T) {
	a := run.Adapter{Bin: stub(t, "echo 'the harness fell over' >&2\nexit 2")}

	s, err := a.Sit(context.Background(), work(t))
	if err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if !strings.Contains(s.Stderr, "the harness fell over") {
		t.Errorf("Stderr = %q, want the harness's own message", s.Stderr)
	}
	if s.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", s.ExitCode)
	}
}

// TestSitBoundsTheStderrItKeeps. The harness is held only by a wall-clock cap,
// and a chatty one under debugging flags can produce output without limit.
func TestSitBoundsTheStderrItKeeps(t *testing.T) {
	// Past the bound, and distinguishable end to end: keeping the head rather
	// than the tail is the plausible mistake, and identical lines would hide it.
	a := run.Adapter{Bin: stub(t,
		"echo 'FIRSTLINE' >&2\n"+
			"i=0; while [ $i -lt 40000 ]; do echo 'sevenxx' >&2; i=$((i+1)); done\n"+
			"echo 'LASTLINE' >&2\nexit 1")}

	s, err := a.Sit(context.Background(), work(t))
	if err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if len(s.Stderr) > 8<<10 {
		t.Errorf("kept %d bytes of stderr; the bound is 8 KiB", len(s.Stderr))
	}
	if s.Stderr == "" {
		t.Error("kept no stderr at all; the bound is not a mute button")
	}
	if !strings.Contains(s.Stderr, "LASTLINE") {
		t.Error("the last thing the harness said was dropped; when a process " +
			"falls over the reason is at the end, which is why this keeps the tail")
	}
	if strings.Contains(s.Stderr, "FIRSTLINE") {
		t.Error("the first line survived past the bound, so this is keeping the " +
			"head; the doc comment says otherwise")
	}
}

// TestConformance runs the shared kit. It never reaches a process — everything
// it checks terminates before one starts — and the binary named here does not
// exist, so an accidental widening of the kit fails loudly rather than spending
// a contributor's money.
func TestConformance(t *testing.T) {
	conformance.VerifyRun(t, run.Adapter{Bin: filepath.Join(t.TempDir(), "no-such-harness")})
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the recorded argument list: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// hasFlag reports whether args contains flag, followed by value when one is
// wanted.
//
// It matches any element equal to flag, including one sitting in value position.
// That cannot fire against the fixed values these tests pass, and tightening it
// would mean teaching the helper which flags take values — which is the thing
// under test.
func hasFlag(args []string, flag, value string, hasValue bool) bool {
	for i, a := range args {
		if a != flag {
			continue
		}
		if !hasValue {
			return true
		}
		if i+1 < len(args) && args[i+1] == value {
			return true
		}
	}
	return false
}

// TestSitClassifiesTheOtherEndingsTheSchemaPermits covers the results the three
// recorded fixtures do not contain. They are written from the harness's own
// declared schema rather than recorded, and are marked as such: a hand-written
// fixture proves this adapter reads a shape correctly and proves nothing about
// whether the harness produces it.
func TestSitClassifiesTheOtherEndingsTheSchemaPermits(t *testing.T) {
	const denied = `{"type":"result","subtype":"error_during_execution","is_error":true,` +
		`"session_id":"s-1","errors":["blocked"],` +
		`"permission_denials":[{"tool_name":"Bash","tool_use_id":"t1"},{"tool_name":"Write","tool_use_id":"t2"}]}`

	tests := []struct {
		name        string
		body        string
		exit        int
		want        assay.StopReason
		wantDenials []string
	}{
		{
			name: "refused its tools",
			body: denied,
			exit: 1,
			want: assay.StopDenied,
			// Both reach ERROR either way; naming the denial points at the
			// exam's allowlist, which is the thing worth fixing.
			wantDenials: []string{"Bash", "Write"},
		},
		{
			name: "fell over with nothing refused",
			body: `{"type":"result","subtype":"error_during_execution","is_error":true,` +
				`"session_id":"s-2","errors":["boom"],"permission_denials":[]}`,
			exit: 1,
			want: assay.StopFailed,
		},
		{
			name: "ran out of retries",
			body: `{"type":"result","subtype":"error_max_structured_output_retries",` +
				`"is_error":true,"session_id":"s-3","permission_denials":[]}`,
			exit: 1,
			want: assay.StopFailed,
		},
		{
			name: "success flagged as an error",
			body: `{"type":"result","subtype":"success","is_error":true,` +
				`"session_id":"s-4","result":"done","permission_denials":[]}`,
			exit: 0,
			want: assay.StopUnknown,
		},
		{
			name: "a subtype from a later release",
			body: `{"type":"result","subtype":"error_something_new","is_error":true,` +
				`"session_id":"s-5","permission_denials":[]}`,
			exit: 1,
			want: assay.StopUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := run.Adapter{Bin: stub(t, "printf '%s' '"+tt.body+"'\nexit "+strconv.Itoa(tt.exit))}

			s, err := a.Sit(context.Background(), work(t))
			if err != nil {
				t.Fatalf("Sit: %v", err)
			}
			if s.Stop != tt.want {
				t.Errorf("Stop = %v, want %v", s.Stop, tt.want)
			}
			if s.Stop.Decides() {
				t.Error("this ending permits a verdict; only a completed run may")
			}
			if !reflect.DeepEqual(s.Denials, tt.wantDenials) {
				t.Errorf("Denials = %q, want %q", s.Denials, tt.wantDenials)
			}
		})
	}
}

// TestAnAdapterWithNoBinaryNamedLooksOnThePath covers the branch every caller
// will actually take. A composition root constructs this adapter with no Bin,
// and until now the only untested statement in it was the line deciding what to
// run — so a typo in the harness's name would have shipped.
//
// The stub goes on PATH rather than being named directly, which is the whole
// point: the lookup is the thing under test.
func TestAnAdapterWithNoBinaryNamedLooksOnThePath(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("testdata", completed))
	if err != nil {
		t.Fatal(err)
	}
	bin := stub(t, "cat '"+abs+"'")
	// stub names the file "claude" already; put its directory first on the path.
	// Prepended rather than substituted: the stub is a shell script that runs
	// cat, and the adapter hands its own PATH through to the harness, so
	// replacing it outright leaves the stub unable to find its own tools.
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	s, err := run.Adapter{}.Sit(context.Background(), work(t))
	if err != nil {
		t.Fatalf("Sit with no binary named: %v", err)
	}
	if s.Stop != assay.StopComplete {
		t.Errorf("Stop = %v, want complete", s.Stop)
	}
}

// Package run administers sittings on Claude Code's headless mode.
//
// It is the run half of the reference adapter pair; the capture half reads what
// this produces. Like that half, it is one of the few places allowed to know a
// harness exists, and everything downstream works from what it returns without
// knowing where it came from.
//
// What it guarantees was measured against the binary rather than read off its
// help text, and the two disagree. The turn cap this adapter relies on is a real
// flag that the help does not list; both caps stop a run and neither bounds one.
// The details are on Guarantees and on the flags they map to.
package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// Adapter drives the Claude Code command line in print mode.
type Adapter struct {
	// Bin is the harness executable. Empty means "claude", found on the path.
	Bin string
}

var _ port.Run = Adapter{}

// ID implements port.Run. It matches the capture adapter's identifier: the two
// halves describe the same harness, and a report naming one names the other.
func (Adapter) ID() string { return "claude-code" }

// Tier implements port.Run. The harness has a documented headless mode that
// reports its own result, which is the strongest way to drive one.
func (Adapter) Tier() assay.Tier { return assay.TierNative }

// Guarantees implements port.Run.
//
// Every value here was established by running the binary, and two of them
// contradict what its documentation implies:
//
//   - Turns is native. The help output on 2.1.220 does not list --max-turns, and
//     an earlier draft of this adapter declared turn caps unenforceable on that
//     basis. The flag exists: a measured run capped at one turn came back
//     subtype error_max_turns, terminal_reason max_turns. It is relied on and
//     the reliance is pinned, because a flag absent from the help is a flag
//     nobody promised to keep.
//
//   - Budget is native and does not bound spending. A measured run capped at
//     $0.001 cost $0.0047. The harness stops when it notices the threshold
//     crossed, which is after the turn that crossed it has been paid for.
//
// Wall is supervised: the harness has no timeout flag, and this adapter owns the
// process, so it holds that cap itself by ending it. That is the weaker
// guarantee — a killed harness reports nothing about what it was doing — and it
// is declared as the weaker one.
func (Adapter) Guarantees() assay.Guarantees {
	return assay.Guarantees{
		Budget:   assay.EnforcementNative,
		Turns:    assay.EnforcementNative,
		Wall:     assay.EnforcementSupervised,
		Model:    true,
		Tools:    true,
		Settings: true,
	}
}

// stderrTail is how much of the harness's stderr travels with a sitting.
//
// The tail rather than the head: when a process falls over, the reason is the
// last thing it said. A bound rather than none at all, because a harness run
// under debugging flags can produce output without limit and this one is held
// only by a wall-clock cap.
const stderrTail = 8 << 10

// Sit implements port.Run.
func (a Adapter) Sit(ctx context.Context, as *assay.Assignment) (assay.Sitting, error) {
	if err := port.Refuse(a.Guarantees(), as); err != nil {
		return assay.Sitting{}, err
	}

	bin := a.Bin
	if bin == "" {
		bin = "claude"
	}

	// --bare skips every ambient source of configuration; --setting-sources
	// with an empty list skips the settings files it does not cover. Together
	// they are the hermeticity keystone: the sitting sees exactly the
	// configuration the assignment names and nothing the machine happens to
	// have lying about. --fallback-model is deliberately absent rather than
	// disabled — it is opt-in on this binary, so not passing it is off, and
	// passing it would let the run silently change the model under examination.
	args := []string{
		"--print",
		"--bare",
		"--output-format", "json",
		"--strict-mcp-config",
		"--setting-sources", "",
	}
	if as.Model != "" {
		args = append(args, "--model", as.Model)
	}
	if len(as.Tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(as.Tools, ","))
	}
	if as.Settings != "" {
		args = append(args, "--settings", as.Settings)
	}
	if as.Caps.BudgetMicroUSD > 0 {
		args = append(args, "--max-budget-usd", formatUSD(as.Caps.BudgetMicroUSD))
	}
	if as.Caps.Turns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(as.Caps.Turns))
	}

	runCtx := ctx
	if as.Caps.Wall > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, as.Caps.Wall)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = as.Dir
	// The instruction goes on stdin, not in the argument list. A distilled
	// instruction folds in every mid-session correction and has no bound; an
	// argument list does.
	cmd.Stdin = strings.NewReader(as.Instruction)
	var stdout bytes.Buffer
	stderr := &tailWriter{limit: stderrTail}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	started := time.Now()
	runErr := cmd.Run()
	wall := time.Since(started)

	sitting := assay.Sitting{
		Adapter:    a.ID(),
		Tier:       a.Tier(),
		Guarantees: a.Guarantees(),
		Stderr:     stderr.String(),
		ExitCode:   exitCode(cmd),
	}
	sitting.Usage.Wall = wall

	// Cancellation by the caller is the caller's business and produces no
	// sitting. Cancellation by this adapter's own wall cap produces one, because
	// a sitting stopped by a cap is a result about the run.
	if err := ctx.Err(); err != nil {
		return assay.Sitting{}, err
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		sitting.Stop = assay.StopWall
		return sitting, nil
	}

	var execErr *exec.Error
	if errors.As(runErr, &execErr) || errors.Is(runErr, fs.ErrNotExist) || errors.Is(runErr, fs.ErrPermission) {
		return assay.Sitting{}, fmt.Errorf("%w: %w", port.ErrUnavailable, runErr)
	}

	var res result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil || res.Type != "result" {
		// Output that does not parse leaves nothing to say. A harness that also
		// exited non-zero at least said that much; one that exited cleanly and
		// wrote something unreadable is a case this adapter cannot classify, and
		// says so rather than guessing.
		sitting.Stop = assay.StopFailed
		if runErr == nil {
			sitting.Stop = assay.StopUnknown
		}
		return sitting, nil
	}

	sitting.Native = res.SessionID
	sitting.Denials = res.denials()
	sitting.Stop = res.stop()
	sitting.Usage.CostMicroUSD = int64(math.Round(res.TotalCostUSD * 1e6))
	// Every input token the harness billed for, cached or not, matching what the
	// capture side counts. On the store measured there, cached input was 99.97%
	// of the total; reading the uncached field alone reports a run three orders
	// of magnitude cheaper than it was.
	sitting.Usage.InputTokens = res.Usage.InputTokens +
		res.Usage.CacheCreationInputTokens + res.Usage.CacheReadInputTokens
	sitting.Usage.OutputTokens = res.Usage.OutputTokens

	return sitting, nil
}

// result is the object print mode writes when asked for JSON output.
//
// Only the fields with a consumer are here. The harness writes a good deal more
// — timing breakdowns, per-model usage, fast-mode state — and reading a field
// because it exists is how a neutral representation acquires a harness's shape.
type result struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	IsError      bool    `json:"is_error"`
	SessionID    string  `json:"session_id"`
	TotalCostUSD float64 `json:"total_cost_usd"`

	// The cache fields are nullable in the harness's own schema. Null decodes to
	// zero here, which is the right reading: no cache activity.
	Usage struct {
		InputTokens              int64 `json:"input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
	} `json:"usage"`

	PermissionDenials []struct {
		ToolName string `json:"tool_name"`
	} `json:"permission_denials"`
}

// stop maps the harness's own account of why it stopped.
//
// The subtype is the field to read, and the exit status is not: measured runs
// that hit the turn cap and the budget cap both exited 1, which is also what a
// harness returns when it has genuinely fallen over.
func (r *result) stop() assay.StopReason {
	switch r.Subtype {
	case "success":
		if r.IsError {
			// Success with the error flag set is a combination the schema
			// permits and nothing observed produced. Refusing to read it as a
			// completed run is the safe direction: the cost of being wrong here
			// is an ERROR where a verdict was available, not a verdict that
			// should not exist.
			return assay.StopUnknown
		}
		return assay.StopComplete
	case "error_max_turns":
		return assay.StopTurns
	case "error_max_budget_usd":
		return assay.StopBudget
	case "error_during_execution":
		// Both of these reach ERROR and neither can reach a verdict, so the
		// choice between them changes only the reason a report prints. A failure
		// during execution alongside refused tools is much more likely to be the
		// exam's allowlist than the harness breaking, and saying so points at
		// the thing worth fixing.
		if len(r.PermissionDenials) > 0 {
			return assay.StopDenied
		}
		return assay.StopFailed
	case "error_max_structured_output_retries":
		return assay.StopFailed
	}
	return assay.StopUnknown
}

func (r *result) denials() []string {
	if len(r.PermissionDenials) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.PermissionDenials))
	for _, d := range r.PermissionDenials {
		out = append(out, d.ToolName)
	}
	return out
}

// formatUSD renders integer millionths of a dollar as the decimal the flag
// takes. Money is carried as an integer everywhere else for the reason given on
// Usage; this is the one place it has to become a string a command line accepts.
func formatUSD(microUSD int64) string {
	return strconv.FormatFloat(float64(microUSD)/1e6, 'f', -1, 64)
}

func exitCode(cmd *exec.Cmd) int {
	if cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}

// tailWriter keeps the last limit bytes written to it and discards the rest.
type tailWriter struct {
	buf   []byte
	limit int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.limit {
		w.buf = w.buf[len(w.buf)-w.limit:]
	}
	return len(p), nil
}

func (w *tailWriter) String() string { return string(w.buf) }

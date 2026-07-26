package arch

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// vendorWords are strings with no ordinary programming meaning — strings that
// would only appear in source because someone is naming a harness or its vendor.
//
// The list grows when work on a vendor's adapter BEGINS, not when it merges:
// the window in which coupling can be introduced opens with the first design
// document, not with the pull request. It is seeded with the vendors the
// roadmap names, which is what makes the rule and the list consistent.
//
// "cursor" is deliberately absent. It is an ordinary noun that parsers and
// paginated APIs use legitimately, and a false positive here breaks a build
// rather than adding noise to a report. When a Cursor adapter is written it is
// matched as a qualified pair — cursor adjacent to agent, cli or ide — not as a
// bare noun. The phase 0 redaction work already showed what an unmeasured
// detector costs: it caught every planted secret and still could not be used as
// a gate, because it fired on one local file in two.
var vendorWords = []string{"anthropic", "claude", "codex", "gemini", "openai"}

// vocabExempt are the only paths where a vendor word is legal.
//
//   - internal/adapter/ is the entire purpose of an adapter.
//   - internal/app/adapters.go is the registration table: the composition root
//     must name what it wires. TestAdapterRegistryHasNoControlFlow keeps that
//     file a table rather than a place for decisions.
//   - internal/arch/ is this guard. The word list above lives here as string
//     literals, so without this exemption the scan flags its own configuration.
//
// Raising this count means amending ADR-0005, not editing a number.
var vocabExempt = []string{
	"internal/adapter/",
	"internal/app/adapters.go",
	"internal/arch/",
}

// TestExemptionCountIsFixed turns adding a fourth exemption from an append into
// a visible decision. The exemptions are the guard's only holes; they should be
// hard to widen by accident.
func TestExemptionCountIsFixed(t *testing.T) {
	const want = 3
	if len(vocabExempt) != want {
		t.Fatalf("vocabExempt has %d entries, want %d.\n"+
			"  Adding an exemption widens the only hole in the vocabulary scan.\n"+
			"  Amend docs/adr/0005-internal-package-layout.md first, then change this number.",
			len(vocabExempt), want)
	}
}

// isVocabExempt reports whether a repo-relative path may name a vendor.
func isVocabExempt(relPath string) bool {
	for _, ex := range vocabExempt {
		if strings.HasPrefix(relPath, ex) || relPath == strings.TrimSuffix(ex, "/") {
			return true
		}
	}
	return false
}

// containsVendorWord reports every vendor word inside s.
//
// Matching is case-insensitive SUBSTRING — not whole-word, not segment-wise.
// Segment matching was specified first and is wrong: "claudecode" is one
// segment and is not equal to "claude", so it would pass while "claude-code"
// fails. "claudecode" is this project's own directory spelling for the
// reference adapter, and therefore the spelling a contributor will have typed
// most often. A rule that gives opposite answers to two spellings of one word
// is worse than no rule, because it reads as protection.
//
// The only English collisions across this list are "philanthropic" and
// "misanthropic", whose cost is one rename.
func containsVendorWord(s string) []string {
	lower := strings.ToLower(s)
	var hits []string
	for _, w := range vendorWords {
		if strings.Contains(lower, w) {
			hits = append(hits, w)
		}
	}
	return hits
}

// findVendorWordsInGo scans a parsed Go file's identifiers and string literals.
//
// Comments are NOT scanned. Prose may reasonably name a harness — an adapter's
// rationale, a note about why something is neutral — and prose cannot branch.
func findVendorWordsInGo(f *ast.File) []string {
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			hits = append(hits, containsVendorWord(v.Name)...)
		case *ast.BasicLit:
			hits = append(hits, containsVendorWord(v.Value)...)
		}
		return true
	})
	return hits
}

// TestNoVendorVocabulary scans every file under the scan roots, not only Go
// source.
//
// An embedded defaults.yaml holding a harness's transcript path is ordinary Go
// — //go:embed is a comment, the filename is innocent, and the content would
// otherwise be an unwatched surface sitting directly beside core.
//
// This exists as well as the import check because the failure it prevents is
// import-free: comparing against a harness name in a shared package imports
// nothing and satisfies any import-based rule.
func TestNoVendorVocabulary(t *testing.T) {
	root := repoRoot(t)

	// Index the parsed Go files once rather than reparsing per file.
	parsed := map[string]*ast.File{}
	for _, f := range goFiles(t, root) {
		parsed[f.Path] = f.AST
	}

	walkFiles(t, root, func(rel string, d fs.DirEntry) error {
		if isVocabExempt(rel) {
			return nil
		}

		for _, w := range containsVendorWord(d.Name()) {
			t.Errorf("%s: filename contains vendor word %q", rel, w)
		}

		if strings.HasSuffix(rel, ".go") {
			if f, ok := parsed[rel]; ok {
				for _, w := range findVendorWordsInGo(f) {
					t.Errorf("%s: identifier or string literal contains vendor word %q.\n"+
						"  Only internal/adapter/** and internal/app/adapters.go may name a harness.",
						rel, w)
				}
			}
			return nil
		}

		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return err
		}
		for _, w := range containsVendorWord(string(src)) {
			t.Errorf("%s: file contents contain vendor word %q.\n"+
				"  Embedded configuration beside core is scanned like core.", rel, w)
		}
		return nil
	})
}

// TestVendorScanCatchesBothSpellings is the regression test for the bug that
// segment matching would have shipped. "claude-code" and "claudecode" must both
// be caught; a segment-wise rule caught only the first, and the second is this
// project's own spelling.
func TestVendorScanCatchesBothSpellings(t *testing.T) {
	tests := []struct {
		fixture string
		why     string
	}{
		{"vendorliteral.go.txt", `a hyphenated string literal: "claude-code"`},
		{"vendorident.go.txt", "a camel-cased identifier with no separator: claudecodeQuirk"},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			if hits := findVendorWordsInGo(parseFixture(t, tt.fixture)); len(hits) == 0 {
				t.Errorf("scan found nothing in %s, want at least one hit — %s", tt.fixture, tt.why)
			}
		})
	}
}

// TestVendorScanExemptions pins which paths may and may not name a vendor.
func TestVendorScanExemptions(t *testing.T) {
	exempt := []string{
		"internal/adapter/claudecode/capture/parse.go",
		"internal/adapter/wire/capture/proxy.go",
		"internal/app/adapters.go",
		"internal/arch/vocab_test.go",
	}
	for _, p := range exempt {
		if !isVocabExempt(p) {
			t.Errorf("%s should be exempt from the vocabulary scan", p)
		}
	}

	notExempt := []string{
		"internal/examiner/examiner.go",
		"internal/assay/session.go",
		"internal/app/app.go",
		"internal/cli/cli.go",
		"cmd/assayer/main.go",
	}
	for _, p := range notExempt {
		if isVocabExempt(p) {
			t.Errorf("%s must not be exempt from the vocabulary scan", p)
		}
	}
}

// TestVendorWordsHaveNoOrdinaryMeaning documents the rule the list obeys, so a
// later addition has to confront it. Words that are also ordinary programming
// nouns are matched as vendor-qualified pairs when their adapter is written,
// never added here bare.
func TestVendorWordsHaveNoOrdinaryMeaning(t *testing.T) {
	banned := map[string]string{
		"cursor":  "an ordinary noun: parsers and paginated APIs have cursors",
		"agent":   "an ordinary noun, and this project's whole subject",
		"session": "an ordinary noun, and a core domain type",
		"code":    "an ordinary noun",
		"api":     "an ordinary noun",
	}
	for _, w := range vendorWords {
		if why, bad := banned[w]; bad {
			t.Errorf("vendorWords contains %q, which is %s.\n"+
				"  Match it as a vendor-qualified pair instead.", w, why)
		}
	}

	if !sort.StringsAreSorted(vendorWords) {
		// Not correctness, but a sorted list makes an addition obvious in review.
		t.Logf("vendorWords is unsorted: %v", vendorWords)
	}
}

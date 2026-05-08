package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build binary
	tmp, err := os.CreateTemp("", "unid-test-*")
	if err != nil {
		panic(err)
	}
	tmp.Close()
	binaryPath = tmp.Name()

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/unid")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("build failed: " + string(out))
	}

	code := m.Run()
	os.Remove(binaryPath)
	os.Exit(code)
}

func runStdin(input string) (stdout, stderr string, ok bool) {
	cmd := exec.Command(binaryPath)
	cmd.Stdin = strings.NewReader(input)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err == nil
}

func runSubcmd(subcmd, input string) (stdout, stderr string, ok bool) {
	cmd := exec.Command(binaryPath, subcmd)
	cmd.Stdin = strings.NewReader(input)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err == nil
}

// ─── Golden file tests ──────────────────────────────────────────────

func TestGoldenFiles(t *testing.T) {
	dslFiles, err := filepath.Glob("tests/fixtures/*.dsl")
	require.NoError(t, err)
	require.NotEmpty(t, dslFiles, "no .dsl fixture files found")

	for _, dslPath := range dslFiles {
		goldenPath := strings.TrimSuffix(dslPath, ".dsl") + ".golden"
		name := strings.TrimSuffix(filepath.Base(dslPath), ".dsl")

		t.Run(name, func(t *testing.T) {
			dsl, err := os.ReadFile(dslPath)
			require.NoError(t, err)

			expected, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "missing golden file: %s", goldenPath)

			stdout, stderr, ok := runStdin(string(dsl))
			require.True(t, ok, "render failed for %s: %s", name, stderr)
			assert.Equal(t, string(expected), stdout)
		})
	}
}

// ─── Collision ───────────────────────────────────────────────────────

func TestCollisionOnError(t *testing.T) {
	_, stderr, ok := runStdin("collision on\nbox 0 0 5 1\nbox 3 0 5 1")
	assert.False(t, ok)
	assert.Contains(t, stderr, "collision")
}

func TestCollisionErrorFormat(t *testing.T) {
	_, stderr, ok := runStdin("collision on\nbox 0 0 5 1\nbox 3 0 5 1")
	assert.False(t, ok)
	assert.Contains(t, stderr, "object #2")
	assert.Contains(t, stderr, "object #1")
	assert.Contains(t, stderr, "overlaps")
	assert.Contains(t, stderr, "size")
}

// ─── Content Overflow ────────────────────────────────────────────────

func TestOverflowError(t *testing.T) {
	_, stderr, ok := runStdin("collision off\nbox 0 0 3 1 overflow=error c=VeryLong")
	assert.False(t, ok)
	assert.Contains(t, stderr, "overflow")
}

// ─── List ────────────────────────────────────────────────────────────

func TestListSubcommand(t *testing.T) {
	stdout, _, ok := runSubcmd("list", "collision on\nbox 0 0 8 1 c=Box\ntext 15 1 c=Hi")
	require.True(t, ok)
	assert.Contains(t, stdout, "Canvas:")
	assert.Contains(t, stdout, "Collision: on")
	assert.Contains(t, stdout, "Objects: 2")
	assert.Contains(t, stdout, "box")
	assert.Contains(t, stdout, "text")
}

func TestListAutoCanvas(t *testing.T) {
	stdout, _, ok := runSubcmd("list", "collision off\nbox 0 0 4 1")
	require.True(t, ok)
	assert.Contains(t, stdout, "(auto)")
}

// ─── Lint ────────────────────────────────────────────────────────────

func TestLintOk(t *testing.T) {
	stdout, _, ok := runSubcmd("lint", "collision off\nbox 0 0 4 1")
	require.True(t, ok)
	assert.Contains(t, stdout, "OK")
}

func TestLintCollisionError(t *testing.T) {
	stdout, _, ok := runSubcmd("lint", "collision on\nbox 0 0 5 1\nbox 3 0 5 1")
	assert.False(t, ok)
	assert.Contains(t, stdout, "Errors:")
	assert.Contains(t, stdout, "collision")
}

// ─── Guide ──────────────────────────────────────────────────────────

func TestGuideSubcommand(t *testing.T) {
	cmd := exec.Command(binaryPath, "guide")
	out, err := cmd.Output()
	require.NoError(t, err)
	stdout := string(out)
	assert.Contains(t, stdout, "USAGE:")
	assert.Contains(t, stdout, "DSL SYNTAX:")
	assert.Contains(t, stdout, "BOX STYLES")
}

// ─── Error cases ────────────────────────────────────────────────────

func TestErrorMissingCollision(t *testing.T) {
	_, stderr, ok := runStdin("box 0 0 4 1")
	assert.False(t, ok)
	assert.Contains(t, stderr, "collision")
}

func TestErrorParseError(t *testing.T) {
	_, stderr, ok := runStdin("collision on\nbadcmd 1 2")
	assert.False(t, ok)
	assert.Contains(t, stderr, "unknown command")
}

func TestErrorUnknownArrowID(t *testing.T) {
	_, stderr, ok := runStdin("collision off\nbox 0 0 4 1 id=a\narrow a.r nonexistent.l")
	assert.False(t, ok)
	assert.Contains(t, stderr, "unknown object id")
}

func TestErrorInvalidArrowAnchor(t *testing.T) {
	_, stderr, ok := runStdin("collision off\narrow noid db.top")
	assert.False(t, ok)
	assert.Contains(t, stderr, "invalid anchor")
}

func TestDuplicateIDError(t *testing.T) {
	_, stderr, ok := runStdin("collision off\nbox 0 0 4 1 id=a\nbox 10 0 4 1 id=a")
	assert.False(t, ok)
	assert.Contains(t, stderr, "duplicate")
}

func TestArrowInvalidArrowheadRejected(t *testing.T) {
	_, stderr, ok := runStdin("collision off\nbox 0 0 4 1 id=a c=A\nbox 20 0 4 1 id=b c=B\narrow a.r b.l head=◆")
	assert.False(t, ok)
	assert.Contains(t, stderr, "invalid arrowhead")
}

func TestRectLegendLRError(t *testing.T) {
	_, stderr, ok := runStdin("collision off\nbox 0 0 10 1 legend-pos=l lg=Bad")
	assert.False(t, ok)
	assert.Contains(t, stderr, "legend-pos only supports top")
}

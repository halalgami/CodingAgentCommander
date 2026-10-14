package scan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkRepo(b *testing.B) {
	root := b.TempDir()
	gitB(b, root, "init", "-q")
	body := "package x\n" + strings.Repeat("func f(a bool) { if a && a { for {} } }\n", 50)
	for i := 0; i < 2000; i++ {
		writeB(b, root, fmt.Sprintf("p%02d/f%04d.go", i%40, i), body)
	}
	gitB(b, root, "add", ".")
	gitB(b, root, "commit", "-qm", "x")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Repo(context.Background(), root, DefaultLimits, now); err != nil {
			b.Fatal(err)
		}
	}
}

func gitB(b *testing.B, dir string, args ...string) {
	b.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git %v: %v %s", args, err, out)
	}
}

func writeB(b *testing.B, root, rel, body string) {
	b.Helper()
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		b.Fatal(err)
	}
}

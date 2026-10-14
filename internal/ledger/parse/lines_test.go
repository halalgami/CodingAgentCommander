package parse

import (
	"strings"
	"testing"
)

func TestReadLinesCompleteOnlyAndSkipsHuge(t *testing.T) {
	huge := strings.Repeat("y", MaxLine+10)
	in := "a\n" + huge + "\n" + "b\n" + "partial"
	var got []string
	var total int64
	consumed, skipped, err := ReadLines(strings.NewReader(in), func(line []byte, n int64) {
		total += n
		if line == nil {
			got = append(got, "<skipped>")
			return
		}
		got = append(got, strings.TrimSuffix(string(line), "\n"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "<skipped>", "b"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("lines = %v, want %v", got, want)
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d", skipped)
	}
	if wantConsumed := int64(len(in) - len("partial")); consumed != wantConsumed || total != wantConsumed {
		t.Fatalf("consumed=%d total=%d want %d (partial tail must stay unconsumed)", consumed, total, wantConsumed)
	}
}

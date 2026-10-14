package parse

import (
	"bufio"
	"io"
)

// ReadLines calls fn for every COMPLETE line ('\n'-terminated) readable from r.
// n is the line's byte length including '\n'. A line longer than MaxLine is
// reported as fn(nil, n): consumed, never buffered. A trailing partial line is
// not consumed (the writer is mid-append; the next read sees it whole).
func ReadLines(r io.Reader, fn func(line []byte, n int64)) (consumed, skipped int64, err error) {
	br := bufio.NewReaderSize(r, 256<<10)
	var scratch []byte
	for {
		b, e := br.ReadSlice('\n')
		n := int64(len(b))
		over := false
		if e == bufio.ErrBufferFull {
			scratch = append(scratch[:0], b...)
			for e == bufio.ErrBufferFull {
				b, e = br.ReadSlice('\n')
				n += int64(len(b))
				if !over && len(scratch)+len(b) <= MaxLine {
					scratch = append(scratch, b...)
				} else {
					over = true
					scratch = scratch[:0]
				}
			}
			b = scratch
		}
		if e == nil && n > 0 {
			consumed += n
			if over {
				skipped++
				fn(nil, n)
			} else {
				fn(b, n)
			}
			continue
		}
		if e == io.EOF {
			return consumed, skipped, nil
		}
		return consumed, skipped, e
	}
}

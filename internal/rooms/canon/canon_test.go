package canon

import "testing"

func TestMarshalIsOrderIndependentAndCompact(t *testing.T) {
	a := map[string]any{"b": 1, "a": []any{map[string]any{"y": "é<>&", "x": true}}, "n": 9007199254740993}
	b := map[string]any{"n": 9007199254740993, "a": []any{map[string]any{"x": true, "y": "é<>&"}}, "b": 1}
	x, err := Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	y, _ := Marshal(b)
	want := `{"a":[{"x":true,"y":"é<>&"}],"b":1,"n":9007199254740993}`
	if string(x) != want || string(y) != want {
		t.Fatalf("got %s / %s", x, y)
	}
}

type st struct {
	Z string `json:"z"`
	A int64  `json:"a"`
	O string `json:"o,omitempty"`
}

func TestMarshalStructSortsKeys(t *testing.T) {
	got, err := Marshal(st{Z: "z", A: 7})
	if err != nil || string(got) != `{"a":7,"z":"z"}` {
		t.Fatalf("%s %v", got, err)
	}
}

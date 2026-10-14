// Package raidpack holds user-supplied Raid heroine art: which file names a
// pack may contain, its optional manifest, loading, image normalising, import
// and the starter kit. Pure: no Wails, no app state.
package raidpack

import "regexp"

var (
	IDs   = []string{"opus", "sonnet", "haiku", "glm", "qwen", "kimi"}
	Poses = []string{"attack", "await", "hurt", "stance", "victory"}

	heroStem = regexp.MustCompile(`^(opus|sonnet|haiku|glm|qwen|kimi)(-v1|-v2|-legend)?-(attack|await|hurt|stance|victory)$`)
	gearStem = regexp.MustCompile(`^gear-familiar-(opus|sonnet|haiku|glm|qwen|kimi)$`)
	capStem  = regexp.MustCompile(`^captain-(opus|sonnet|haiku|glm|qwen|kimi)$`) // Fleet captain portrait
)

// ImageExt are the extensions a pack image may have.
var ImageExt = map[string]bool{".png": true, ".webp": true, ".jpg": true, ".jpeg": true}

func ValidStem(s string) bool {
	return heroStem.MatchString(s) || gearStem.MatchString(s) || capStem.MatchString(s)
}

// ValidID reports whether id is one of the heroine ids (IDs).
func ValidID(id string) bool { return validID(id) }

func validID(id string) bool {
	for _, x := range IDs {
		if x == id {
			return true
		}
	}
	return false
}

// AllStems lists every stem a pack can hold, heroine by heroine.
func AllStems() []string {
	var out []string
	for _, id := range IDs {
		for _, v := range []string{"", "-v1", "-v2", "-legend"} {
			for _, p := range Poses {
				out = append(out, id+v+"-"+p)
			}
		}
		out = append(out, "gear-familiar-"+id, "captain-"+id)
	}
	return out
}

package safepath

import (
	"path/filepath"
	"testing"
)

func TestCleanRel(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"/a/b/":      "a/b",
		"../../etc":  "etc",
		"a/../../b":  "a/b",
		`a\..\b`:     "a/b",
		" 课件 / 第一章 ": "课件/第一章",
	}
	for in, want := range cases {
		if got := CleanRel(in); got != want {
			t.Errorf("CleanRel(%q)=%q want %q", in, got, want)
		}
	}
}

func TestJoinStaysInside(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"", "a", "../x", "a/../../x", "/abs"} {
		abs, ok := Join(root, rel)
		if !ok {
			t.Errorf("Join(%q) not ok: %s", rel, abs)
		}
	}
}

func TestWithinRejectsSiblingPrefix(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Fire")
	if Within(root, root+"2") {
		t.Fatal("Fire2 must not be inside Fire")
	}
	if !Within(root, filepath.Join(root, "sub", "f.txt")) {
		t.Fatal("child must be inside")
	}
}

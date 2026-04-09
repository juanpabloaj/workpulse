package collect

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseBatchCWDOutput(t *testing.T) {
	out := strings.Join([]string{
		"p101",
		"fcwd",
		"n/home/user/src/project1",
		"p202",
		"f1",
		"n/dev/null",
		"fcwd",
		"n/home/user/src/project2",
		"f2",
		"n/tmp/other",
		"p303",
		"",
	}, "\n")

	got := parseBatchCWDOutput(out)
	want := map[int]string{
		101: "/home/user/src/project1",
		202: "/home/user/src/project2",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseBatchCWDOutput() = %#v, want %#v", got, want)
	}
}

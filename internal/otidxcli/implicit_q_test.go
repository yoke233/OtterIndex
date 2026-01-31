package otidxcli

import (
	"reflect"
	"testing"
)

func TestRewriteArgsForImplicitQ(t *testing.T) {
	root := NewRootCommand()

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "empty", in: nil, want: nil},
		{name: "explicit_q", in: []string{"q", "hello"}, want: []string{"q", "hello"}},
		{name: "explicit_index", in: []string{"index", "build", "."}, want: []string{"index", "build", "."}},
		{name: "implicit_query_single", in: []string{"hello"}, want: []string{"q", "hello"}},
		{name: "implicit_query_multi", in: []string{"hello", "world"}, want: []string{"q", "hello", "world"}},
		{name: "implicit_query_with_database_flag", in: []string{"-d", "demo", "hello"}, want: []string{"q", "-d", "demo", "hello"}},
		{name: "explicit_index_with_database_flag", in: []string{"-d", "demo", "index", "build", "."}, want: []string{"-d", "demo", "index", "build", "."}},
		{name: "root_only_flags", in: []string{"--list-databases"}, want: []string{"--list-databases"}},
		{name: "root_viz_flag", in: []string{"--viz", "ascii"}, want: []string{"--viz", "ascii"}},
		{name: "help_command", in: []string{"help"}, want: []string{"help"}},
		{name: "completion_command", in: []string{"completion", "bash"}, want: []string{"completion", "bash"}},
		{name: "dash_dash_keeps_positional", in: []string{"--", "-foo"}, want: []string{"q", "--", "-foo"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RewriteArgsForImplicitQ(root, tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}


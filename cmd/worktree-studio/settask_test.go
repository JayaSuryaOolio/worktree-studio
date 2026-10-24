package main

import "testing"

func TestTaskFlagsSetRef(t *testing.T) {
	for in, want := range map[string]taskFlags{
		"jira:PROJ-1": {Source: "jira", Key: "PROJ-1"},
		"12":          {Source: "local", Key: "12"},
		"#12":         {Source: "local", Key: "12"},
	} {
		var got taskFlags
		got.setRef(in)
		if got != want {
			t.Errorf("setRef(%q) = %+v, want %+v", in, got, want)
		}
	}
}

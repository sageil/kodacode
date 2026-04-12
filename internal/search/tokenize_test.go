package search

import "testing"

func TestSplitTokens(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CheckPermission", "check permission checkpermission"},
		{"HTTPServer", "http server httpserver"},
		{"getHTTPResponse", "get http response gethttpresponse"},
		{"snake_case_func", "snake case func snake_case_func"},
		{"simple", "simple"},
		{"XMLParser", "xml parser xmlparser"},
		{"parseJSON", "parse json parsejson"},
		{"A", "a"},
		{"", ""},
		{"MyHTTPSHandler", "my https handler myhttpshandler"},
		{"already_lower", "already lower already_lower"},
		{"MixedCase_and_snake", "mixed case and snake mixedcase_and_snake"},
		{"io.Reader", "io reader io.reader"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SplitTokens(tt.input)
			if got != tt.want {
				t.Errorf("SplitTokens(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

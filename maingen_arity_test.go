package server

import (
	"os"
	"path/filepath"
	"testing"
)

// Test 3 — the generated routes.Register call matches the arity of the user's
// Register function: the router first, a nil placeholder for every dependency
// after it.
func TestDetectRegisterArgs(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "router only",
			src:  "package routes\nimport \"webtyp.com/router\"\nfunc Register(r router.Router) {}\n",
			want: "s.Router()",
		},
		{
			name: "one dependency",
			src:  "package routes\nimport \"webtyp.com/router\"\nfunc Register(r router.Router, db any) {}\n",
			want: "s.Router(), nil",
		},
		{
			name: "several dependencies, grouped params",
			src:  "package routes\nimport \"webtyp.com/router\"\nfunc Register(r router.Router, a, b any, c any) {}\n",
			want: "s.Router(), nil, nil, nil",
		},
		{
			name: "no Register, first param is router.Router",
			src:  "package routes\nimport \"webtyp.com/router\"\nfunc Wire(r router.Router, mailer any) {}\n",
			want: "s.Router(), nil",
		},
		{
			name: "unparseable file falls back to single arg",
			src:  "package routes\nfunc Register(r router.Router {\n",
			want: "s.Router()",
		},
		{
			name: "missing file falls back to single arg",
			src:  "",
			want: "s.Router()",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "routes.go")
			if tc.src != "" {
				if err := os.WriteFile(path, []byte(tc.src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := detectRegisterArgs(path); got != tc.want {
				t.Errorf("detectRegisterArgs = %q, want %q", got, tc.want)
			}
		})
	}
}

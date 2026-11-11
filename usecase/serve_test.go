package usecase

import "testing"

func TestBindHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{":3000", ""},
		{"0.0.0.0:3000", "0.0.0.0"},
		{"127.0.0.1:0", "127.0.0.1"},
		{"[::1]:3000", "::1"},
		{"localhost:3000", "localhost"},
		{"missing-port", ""},
	}
	for _, c := range cases {
		if got := bindHost(c.in); got != c.want {
			t.Fatalf("bindHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsAnyAddr(t *testing.T) {
	for _, host := range []string{"", "0.0.0.0", "::"} {
		if !isAnyAddr(host) {
			t.Fatalf("%q 应被视为通配地址", host)
		}
	}
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "::1"} {
		if isAnyAddr(host) {
			t.Fatalf("%q 不应被视为通配地址", host)
		}
	}
}

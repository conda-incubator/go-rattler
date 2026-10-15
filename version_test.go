package gorattler

import (
	"strings"
	"sync"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	for _, test := range []struct {
		a, b string
		want int
	}{
		{"1.0rc1", "1.0", -1},
		{"1.0", "1.0", 0},
		{"2.0", "1.0", 1},
		{"1!1.0", "1.99", 1},
	} {
		got, err := CompareVersions(test.a, test.b)
		if err != nil || got != test.want {
			t.Errorf("CompareVersions(%q, %q) = %d, %v; want %d", test.a, test.b, got, err, test.want)
		}
	}
}

func TestCompareVersionsInvalid(t *testing.T) {
	for _, test := range []struct{ a, b, want string }{
		{"", "1", "first"},
		{"1", "", "second"},
		{"invalid version!", "1", "first"},
	} {
		_, err := CompareVersions(test.a, test.b)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("CompareVersions(%q, %q) error = %v; want %q", test.a, test.b, err, test.want)
		}
	}
}

func TestCompareVersionsConcurrent(t *testing.T) {
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 100 {
				if got, err := CompareVersions("1.0rc1", "1.0"); err != nil || got != -1 {
					t.Errorf("concurrent CompareVersions = %d, %v", got, err)
				}
			}
		})
	}
	workers.Wait()
}

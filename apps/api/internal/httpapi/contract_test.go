package httpapi

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const specPath = "../../../../packages/contracts/openapi.yaml"

var (
	specPathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
	specMethodLine = regexp.MustCompile(`^    (get|put|post|delete|patch|head|options|trace):\s*$`)
)

// specOperations returns "METHOD /path" for every operation in the spec.
// It reads the file line by line, relying on the spec's standard two-space
// indentation, to avoid adding a YAML dependency for one test.
func specOperations(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(specPath)
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer f.Close()

	var ops []string
	var current string
	inPaths := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, " ") && line != "" {
			inPaths = line == "paths:"
			continue
		}
		if !inPaths {
			continue
		}
		if m := specPathLine.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if m := specMethodLine.FindStringSubmatch(line); m != nil && current != "" {
			ops = append(ops, strings.ToUpper(m[1])+" "+current)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ops
}

// TestRoutesMatchOpenAPIContract fails when a route is added to or removed
// from the router without updating packages/contracts/openapi.yaml.
func TestRoutesMatchOpenAPIContract(t *testing.T) {
	spec := specOperations(t)
	if len(spec) == 0 {
		t.Fatal("no operations parsed from spec")
	}

	var routes []string
	for _, r := range newTestRouter(fakePinger{}).Routes() {
		routes = append(routes, r.Method+" "+r.Path)
	}

	slices.Sort(spec)
	slices.Sort(routes)
	for _, r := range routes {
		if !slices.Contains(spec, r) {
			t.Errorf("route %s is not documented in %s", r, specPath)
		}
	}
	for _, s := range spec {
		if !slices.Contains(routes, s) {
			t.Errorf("spec operation %s has no route", s)
		}
	}
}

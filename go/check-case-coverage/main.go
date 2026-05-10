// check-case-coverage walks a proto root and verifies every RPC declared
// in *.proto files has at least one .textpb test case in <protoRoot>/cases/
// under <package>.<Service>/<Method>/.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	packageRe = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*;`)
	serviceRe = regexp.MustCompile(`(?m)^\s*service\s+(\w+)\s*\{`)
	rpcRe     = regexp.MustCompile(`(?m)^\s*rpc\s+(\w+)\s*\(`)
)

type rpcID struct{ service, method string }

func (r rpcID) String() string { return r.service + "/" + r.method }

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: check-case-coverage <proto-root>")
		os.Exit(2)
	}
	root := os.Args[1]

	rpcs, err := discoverRPCs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "discover:", err)
		os.Exit(2)
	}
	if len(rpcs) == 0 {
		fmt.Fprintln(os.Stderr, "no RPCs found under", root)
		os.Exit(2)
	}

	casesRoot := filepath.Join(root, "cases")
	var missing []rpcID
	for _, r := range rpcs {
		dir := filepath.Join(casesRoot, r.service, r.method)
		matches, _ := filepath.Glob(filepath.Join(dir, "*.textpb"))
		if len(matches) == 0 {
			missing = append(missing, r)
		}
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing test cases for %d RPC(s):\n", len(missing))
		for _, r := range missing {
			fmt.Fprintf(os.Stderr, "  %s  (expected: %s/%s/*.textpb)\n",
				r, filepath.Join(casesRoot, r.service), r.method)
		}
		os.Exit(1)
	}
	fmt.Printf("ok: %d RPCs each have at least one .textpb case\n", len(rpcs))
}

func discoverRPCs(root string) ([]rpcID, error) {
	var rpcs []rpcID
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip the cases directory and the testing wrapper proto.
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "cases" || base == "testing" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".proto") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rpcs = append(rpcs, parseProto(string(data))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rpcs, func(i, j int) bool { return rpcs[i].String() < rpcs[j].String() })
	return rpcs, nil
}

func parseProto(src string) []rpcID {
	pkg := ""
	if m := packageRe.FindStringSubmatch(src); len(m) == 2 {
		pkg = m[1]
	}
	// Walk service blocks one at a time so we can attribute RPCs to services.
	var out []rpcID
	for {
		svcLoc := serviceRe.FindStringSubmatchIndex(src)
		if svcLoc == nil {
			return out
		}
		svc := src[svcLoc[2]:svcLoc[3]]
		// Find the matching closing brace for this service.
		body, rest := extractBlock(src[svcLoc[1]-1:])
		for _, m := range rpcRe.FindAllStringSubmatch(body, -1) {
			full := svc
			if pkg != "" {
				full = pkg + "." + svc
			}
			out = append(out, rpcID{service: full, method: m[1]})
		}
		src = rest
	}
}

// extractBlock takes a string starting at "{" (after a service header) and
// returns (inside-of-braces, remainder-of-source-after-block).
func extractBlock(s string) (string, string) {
	open := strings.Index(s, "{")
	if open < 0 {
		return "", ""
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[open+1 : i], s[i+1:]
			}
		}
	}
	return s[open+1:], ""
}

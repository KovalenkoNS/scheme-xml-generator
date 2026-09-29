// Command rulescheck audits the actual source inventory required by SDD CODE-001/002.
// It checks package boundaries and comment presence; semantic review remains explicit.
package main

import (
	"fmt"
	"os"
	"sort"
)

// main runs the repository gate for SDD and build scripts and reports concrete paths.
// Any structural or documentation error returns a nonzero status before packaging.
func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if len(os.Args) == 2 {
		root = os.Args[1]
	}
	errors, count := audit(root)
	sort.Strings(errors)
	for _, message := range errors {
		fmt.Println(message)
	}
	if len(errors) > 0 {
		fmt.Printf("Rules gate FAIL: %d issue(s), %d owned Go files inspected.\n", len(errors), count)
		os.Exit(1)
	}
	fmt.Printf("Rules gate PASS: %d owned Go files inspected; package inventory, test separation and production/test comments checked. Semantic review is recorded separately.\n", count)
}

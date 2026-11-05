// Command sqlbundle prints the full database setup SQL.
package main

import (
	"fmt"
	"os"

	"dfipam/api/internal/sqlbundle"
)

func main() {
	out, err := sqlbundle.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sqlbundle:", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

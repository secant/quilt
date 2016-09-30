package stitch

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/scanner"

	"github.com/NetSys/quilt/util"
)

func resolveImports(asts []ast, path string, download bool) ([]ast, error) {
	return resolveImportsRec(asts, path, nil, download)
}

func resolveImportsRec(asts []ast, path string, imported []string,
	download bool) ([]ast, error) {
	var newAsts []ast
	top := true // Imports are required to be at the top of the file.

	for _, ast := range asts {
		name := parseImport(ast)
		if name == "" {
			newAsts = append(newAsts, ast)
			top = false
			continue
		}

		if !top {
			return nil, errors.New(
				"import must be at the beginning of the module")
		}

		// Check for any import cycles.
		for _, importedModule := range imported {
			if name == importedModule {
				return nil, fmt.Errorf("import cycle: %s",
					append(imported, name))
			}
		}

		modulePath := filepath.Join(path, name+".spec")
		var sc scanner.Scanner
		sc.Filename = modulePath
		if _, err := os.Stat(modulePath); os.IsNotExist(err) && download {
			GetSpec(name)
		}

		f, err := util.Open(modulePath)
		if err != nil {
			return nil, fmt.Errorf("unable to open import %s (path=%s)",
				name, modulePath)
		}

		defer f.Close()
		sc.Init(bufio.NewReader(f))
		parsed, err := parse(sc)
		if err != nil {
			return nil, err
		}

		// Rename module name to last name in import path
		name = filepath.Base(name)
		parsed, err = resolveImportsRec(parsed, path, append(imported, name),
			download)
		if err != nil {
			return nil, err
		}

		module := astModule{body: parsed, moduleName: astString(name)}
		newAsts = append(newAsts, module)
	}

	return newAsts, nil
}

func parseImport(ast ast) string {
	sexp, ok := ast.(astSexp)
	if !ok {
		return ""
	}

	if len(sexp.sexp) < 1 {
		return ""
	}

	fnName, ok := sexp.sexp[0].(astBuiltIn)
	if !ok {
		return ""
	}

	if fnName != "import" {
		return ""
	}

	name, ok := sexp.sexp[1].(astString)
	if !ok {
		return ""
	}

	return string(name)
}

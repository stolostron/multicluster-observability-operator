// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type copyright []byte

var stolostron copyright = []byte(`// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

`)

func applyLicenseToProtoAndGo() error {
	return filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Filter out stuff that does not need copyright.
		if info.IsDir() {
			if path == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".deepcopy.go") {
			return nil
		}
		if filepath.Ext(path) != ".proto" && filepath.Ext(path) != ".go" {
			return nil
		}

		b, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}

		if err := writeLicence(stolostron, path, b); err != nil {
			return err
		}
		return nil
	})
}

func writeLicence(cr copyright, path string, b []byte) error {
	if !strings.HasPrefix(string(b), string(cr)) {
		log.Println("file", path, "is missing Copyright header. Adding.")

		var bb bytes.Buffer
		_, _ = bb.Write(cr)
		_, _ = bb.Write(b)
		if err := os.WriteFile(path, bb.Bytes(), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := applyLicenseToProtoAndGo(); err != nil {
		log.Fatal(err)
	}
}

// SPDX-License-Identifier: MIT

// Package gen reads the error-code registry, internal/errs/codes.yaml,
// validates it, renders the Go constants of package errs and
// docs/errors.md from it, and checks how a repository uses the codes.
// Command errsgen is its command-line front end, run by go generate and by
// scripts/check-errors.sh.
//
// The package is used only by the generator and by tests: no irekey binary
// links it, nor the YAML library it depends on.
package gen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// Registry is the content of codes.yaml.
type Registry struct {
	// Categories lists the code prefixes, in the order the codes are
	// listed.
	Categories []Category `yaml:"categories"`
	// Codes lists the registered codes, by category and then by number.
	Codes []Code `yaml:"codes"`
}

// Category is one code prefix and what it covers.
type Category struct {
	Prefix string `yaml:"prefix"`
	Title  string `yaml:"title"`
	Covers string `yaml:"covers"`
	// Line is the line of the entry in codes.yaml, or 0 when unknown.
	Line int `yaml:"-"`
}

// Code is one registered error code. The fields are described in the header
// of codes.yaml.
type Code struct {
	Code        string `yaml:"code"`
	Category    string `yaml:"category"`
	Title       string `yaml:"title"`
	Condition   string `yaml:"condition"`
	Signal      string `yaml:"signal"`
	Handling    string `yaml:"handling"`
	Exit        string `yaml:"exit"`
	DeviceState string `yaml:"device_state"`
	Kind        string `yaml:"kind"`
	Status      string `yaml:"status"`
	Since       string `yaml:"since"`
	// Line is the line of the entry in codes.yaml, or 0 when unknown.
	Line int `yaml:"-"`
}

// ConstName returns the name of the Go constant of a code: the code without
// its dash, for example LOC001 for LOC-001.
func ConstName(code string) string {
	out := make([]byte, 0, len(code))
	for i := range len(code) {
		if code[i] != '-' {
			out = append(out, code[i])
		}
	}
	return string(out)
}

// Load reads and parses a registry file.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	r, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// Parse decodes a registry strictly: an unknown key, a duplicate key, a
// value of the wrong type or a second YAML document is an error. Parse does
// not validate the content; see Validate.
func Parse(data []byte) (*Registry, error) {
	// The document is read twice: as a node tree, which keeps the line of
	// every entry, and into the Registry with unknown keys rejected, which a
	// node tree cannot do.
	var root yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the registry is empty")
		}
		return nil, err
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, errors.New("the registry must be a single YAML document")
	}
	var r Registry
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(&r); err != nil {
		return nil, err
	}
	setLines(&r, &root)
	return &r, nil
}

// setLines records the line of every category and code entry, so that
// validation problems point into the file.
func setLines(r *Registry, root *yaml.Node) {
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return
	}
	top := root.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		key, val := top.Content[i], top.Content[i+1]
		if val.Kind != yaml.SequenceNode {
			continue
		}
		switch key.Value {
		case "categories":
			for j, item := range val.Content {
				if j < len(r.Categories) {
					r.Categories[j].Line = item.Line
				}
			}
		case "codes":
			for j, item := range val.Content {
				if j < len(r.Codes) {
					r.Codes[j].Line = item.Line
				}
			}
		}
	}
}

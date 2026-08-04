package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crossplane/upjet/v2/pkg/registry"
)

func main() {
	root := flag.String("root", "", "directory containing upstream resource Markdown")
	out := flag.String("out", "", "output provider metadata YAML")
	provider := flag.String("provider", "terraform-routeros/routeros", "provider metadata name")
	flag.Parse()
	if *root == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := generate(*root, *out, *provider); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(root, out, provider string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("cannot read RouterOS resource documentation: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	metadata := registry.NewProviderMetadata(provider)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		// root and entry are constrained to the explicitly selected upstream
		// documentation directory and entries returned by os.ReadDir.
		data, err := os.ReadFile(filepath.Join(root, entry.Name())) //nolint:gosec
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", entry.Name(), err)
		}
		resource, err := parseResourceDoc(string(data))
		if err != nil {
			return fmt.Errorf("cannot parse %s: %w", entry.Name(), err)
		}
		if _, exists := metadata.Resources[resource.Name]; exists {
			return fmt.Errorf("duplicate resource documentation for %s", resource.Name)
		}
		metadata.Resources[resource.Name] = resource
	}
	if len(metadata.Resources) == 0 {
		return fmt.Errorf("no RouterOS resource documentation found in %s", root)
	}
	if err := metadata.Store(out); err != nil {
		return fmt.Errorf("cannot store provider metadata: %w", err)
	}
	return nil
}

func parseResourceDoc(document string) (*registry.Resource, error) { //nolint:gocyclo // upstream tfplugindocs sections are parsed in one ordered pass
	lines := strings.Split(document, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty document")
	}
	title := strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
	name := strings.TrimSuffix(title, " (Resource)")
	if !strings.HasPrefix(lines[0], "# routeros_") || name == title {
		return nil, fmt.Errorf("unexpected resource heading %q", lines[0])
	}

	resource := &registry.Resource{
		Name:             name,
		Title:            title,
		ArgumentDocs:     map[string]string{},
		ImportStatements: []string{},
	}
	exampleIndex := indexOf(lines, "## Example Usage")
	var description []string
	inImport := false
	for i, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Example Usage" || trimmed == "## Schema" {
			inImport = false
		}
		if trimmed == "## Import" {
			inImport = true
			continue
		}
		if i > 0 && strings.HasPrefix(trimmed, "## ") {
			inImport = false
		}
		if i < exampleIndex-1 && trimmed != "" && trimmed != "---" {
			description = append(description, trimmed)
		}
		if strings.HasPrefix(trimmed, "- `") {
			rest := strings.TrimPrefix(trimmed, "- `")
			end := strings.Index(rest, "`")
			if end > 0 {
				resource.ArgumentDocs[rest[:end]] = strings.TrimSpace(rest[end+1:])
			}
		}
		if inImport && strings.HasPrefix(trimmed, "terraform import ") {
			resource.ImportStatements = append(resource.ImportStatements, trimmed)
		}
	}
	resource.Description = strings.Join(description, " ")
	return resource, nil
}

func indexOf(lines []string, target string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) == target {
			return i
		}
	}
	return len(lines)
}

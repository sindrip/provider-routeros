package main

import "testing"

func TestParseResourceDoc(t *testing.T) {
	doc := `# routeros_system_script (Resource)
---

Stored script.

## Example Usage

## Schema

### Optional

- ` + "`policy`" + ` (Set of String) List of policies.

## Import

` + "```shell" + `
terraform import routeros_system_script.script "name=example"
` + "```" + `
`
	resource, err := parseResourceDoc(doc)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Name != "routeros_system_script" || resource.Description != "Stored script." {
		t.Fatalf("unexpected resource metadata: %#v", resource)
	}
	if resource.ArgumentDocs["policy"] != "(Set of String) List of policies." {
		t.Fatalf("unexpected policy documentation: %q", resource.ArgumentDocs["policy"])
	}
	if len(resource.ImportStatements) != 1 {
		t.Fatalf("got %d import statements, want 1", len(resource.ImportStatements))
	}
}

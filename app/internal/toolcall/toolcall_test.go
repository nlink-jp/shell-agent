package toolcall

import (
	"os"
	"path/filepath"
	"testing"
)

const testScript = `#!/bin/bash
# @tool: list-files
# @description: List files in a directory
# @param: path string "Directory path to list"
# @category: read

echo "listed"
`

const testWriteScript = `#!/bin/bash
# @tool: write-file
# @description: Write content to a file
# @param: path string "File path"
# @param: content string "Content to write"
# @category: write

cat
`

const notATool = `#!/bin/bash
# This is just a regular script
echo "hello"
`

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeScript(t, dir, "list-files.sh", testScript)
	writeScript(t, dir, "write-file.sh", testWriteScript)
	writeScript(t, dir, "not-a-tool.sh", notATool)

	return dir
}

func writeScript(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryScan(t *testing.T) {
	dir := setupTestDir(t)
	reg := NewRegistry(dir)

	if err := reg.Scan(); err != nil {
		t.Fatal(err)
	}

	tools := reg.List()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
}

func TestRegistryGet(t *testing.T) {
	dir := setupTestDir(t)
	reg := NewRegistry(dir)
	if err := reg.Scan(); err != nil {
		t.Fatal(err)
	}

	tool, ok := reg.Get("list-files")
	if !ok {
		t.Fatal("expected to find list-files tool")
	}
	if tool.Description != "List files in a directory" {
		t.Errorf("unexpected description: %s", tool.Description)
	}
	if len(tool.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(tool.Params))
	}
	if tool.Params[0].Name != "path" {
		t.Errorf("unexpected param name: %s", tool.Params[0].Name)
	}
	if tool.Category != CategoryRead {
		t.Errorf("unexpected category: %s", tool.Category)
	}
}

func TestCategoryMITL(t *testing.T) {
	if CategoryRead.NeedsMITL() {
		t.Error("read should not need MITL")
	}
	if !CategoryWrite.NeedsMITL() {
		t.Error("write should need MITL")
	}
	if !CategoryExecute.NeedsMITL() {
		t.Error("execute should need MITL")
	}
}

func TestRegistryExecute(t *testing.T) {
	dir := setupTestDir(t)
	reg := NewRegistry(dir)
	if err := reg.Scan(); err != nil {
		t.Fatal(err)
	}

	output, err := reg.Execute("list-files", `{"path": "/tmp"}`)
	if err != nil {
		t.Fatal(err)
	}
	if output != "listed\n" {
		t.Errorf("unexpected output: %q", output)
	}
}

func TestRegistryExecuteStdin(t *testing.T) {
	dir := setupTestDir(t)
	reg := NewRegistry(dir)
	if err := reg.Scan(); err != nil {
		t.Fatal(err)
	}

	input := `{"path": "/tmp/test.txt", "content": "hello world"}`
	output, err := reg.Execute("write-file", input)
	if err != nil {
		t.Fatal(err)
	}
	if output != input {
		t.Errorf("expected stdin passthrough, got: %q", output)
	}
}

func TestRegistryExecuteNotFound(t *testing.T) {
	reg := NewRegistry(t.TempDir())
	_, err := reg.Execute("nonexistent", "{}")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestToOpenAITools(t *testing.T) {
	dir := setupTestDir(t)
	reg := NewRegistry(dir)
	if err := reg.Scan(); err != nil {
		t.Fatal(err)
	}

	tools := reg.ToOpenAITools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	for _, tool := range tools {
		if tool["type"] != "function" {
			t.Errorf("expected function type")
		}
		fn, ok := tool["function"].(map[string]interface{})
		if !ok {
			t.Fatal("expected function map")
		}
		if fn["name"] == nil {
			t.Error("expected name")
		}
	}
}

func TestSubdirectoryScanning(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeScript(t, subdir, "nested-tool.sh", testScript)

	reg := NewRegistry(dir)
	if err := reg.Scan(); err != nil {
		t.Fatal(err)
	}

	tools := reg.List()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool from subdirectory, got %d", len(tools))
	}
}

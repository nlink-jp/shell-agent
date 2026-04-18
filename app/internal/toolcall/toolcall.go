package toolcall

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

)

// Category determines MITL behavior.
type Category string

const (
	CategoryRead    Category = "read"
	CategoryWrite   Category = "write"
	CategoryExecute Category = "execute"
)

// NeedsMITL returns true if the category requires user approval.
func (c Category) NeedsMITL() bool {
	return c != CategoryRead
}

// Param describes a tool parameter.
type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolScript represents a registered shell script tool.
type ToolScript struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Params      []Param  `json:"params"`
	Category    Category `json:"category"`
	Path        string   `json:"path"`
}

// Registry discovers and manages tool scripts.
type Registry struct {
	dir   string
	tools map[string]*ToolScript
}

// NewRegistry creates a Registry scanning the given directory.
func NewRegistry(dir string) *Registry {
	return &Registry{
		dir:   dir,
		tools: make(map[string]*ToolScript),
	}
}

// Scan discovers tool scripts by reading header comments.
func (r *Registry) Scan() error {
	r.tools = make(map[string]*ToolScript)
	return filepath.Walk(r.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		tool, err := parseToolHeader(path)
		if err != nil || tool == nil {
			return nil
		}
		r.tools[tool.Name] = tool
		return nil
	})
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (*ToolScript, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []*ToolScript {
	var list []*ToolScript
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// DefaultTimeout is the maximum time a tool script can run.
const DefaultTimeout = 3 * time.Minute

// ExecuteResult holds the output and any blob references from tool execution.
type ExecuteResult struct {
	Output string
	JobID  string
	Blobs  []string // blob references (relative paths)
}

// Execute runs a tool script with JSON input via stdin (legacy, no job).
func (r *Registry) Execute(name string, argsJSON string) (string, error) {
	result, err := r.ExecuteWithJob(name, argsJSON, nil)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// ExecuteWithJob runs a tool script in a job workspace.
// If jobMgr is nil, runs without a workspace.
func (r *Registry) ExecuteWithJob(name string, argsJSON string, jobMgr *JobManager) (*ExecuteResult, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, tool.Path)
	cmd.Stdin = strings.NewReader(argsJSON)

	var job *Job
	if jobMgr != nil {
		var err error
		job, err = jobMgr.NewJob()
		if err != nil {
			return nil, fmt.Errorf("create job: %w", err)
		}
		cmd.Dir = job.WorkDir
		// Pass job info as environment variables
		cmd.Env = append(os.Environ(),
			"SHELL_AGENT_JOB_ID="+job.ID,
			"SHELL_AGENT_WORK_DIR="+job.WorkDir,
		)
	}

	output, err := cmd.Output()
	if err != nil {
		if job != nil {
			_ = os.RemoveAll(job.WorkDir)
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("tool %s timed out after %v", name, DefaultTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("tool %s failed (exit %d): %s", name, exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("tool %s exec error: %w", name, err)
	}

	result := &ExecuteResult{
		Output: string(output),
	}

	// Finalize job: move artifacts to blob storage
	if job != nil && jobMgr != nil {
		result.JobID = job.ID
		blobs, err := jobMgr.Finalize(job)
		if err == nil {
			result.Blobs = blobs
		}
	}

	return result, nil
}

// parseToolHeader reads a script file and extracts @tool annotations.
func parseToolHeader(path string) (*ToolScript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tool := &ToolScript{Path: path, Category: CategoryRead}
	found := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") {
			if found {
				break
			}
			if line != "" && !strings.HasPrefix(line, "#!/") {
				break
			}
			continue
		}

		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "@tool:"):
			tool.Name = strings.TrimSpace(strings.TrimPrefix(line, "@tool:"))
			found = true
		case strings.HasPrefix(line, "@description:"):
			tool.Description = strings.TrimSpace(strings.TrimPrefix(line, "@description:"))
		case strings.HasPrefix(line, "@param:"):
			p := parseParam(strings.TrimSpace(strings.TrimPrefix(line, "@param:")))
			if p != nil {
				tool.Params = append(tool.Params, *p)
			}
		case strings.HasPrefix(line, "@category:"):
			cat := strings.TrimSpace(strings.TrimPrefix(line, "@category:"))
			tool.Category = Category(cat)
		}
	}

	if !found {
		return nil, nil
	}
	return tool, scanner.Err()
}

// parseParam parses a @param line: "name type \"description\""
func parseParam(s string) *Param {
	parts := strings.SplitN(s, " ", 3)
	if len(parts) < 2 {
		return nil
	}
	p := &Param{
		Name: parts[0],
		Type: parts[1],
	}
	if len(parts) == 3 {
		p.Description = strings.Trim(parts[2], "\"")
	}
	return p
}

// ToOpenAITools converts registered tools to OpenAI function tool format.
func (r *Registry) ToOpenAITools() []map[string]interface{} {
	var tools []map[string]interface{}
	for _, t := range r.tools {
		props := make(map[string]interface{})
		var required []string
		for _, p := range t.Params {
			props[p.Name] = map[string]interface{}{
				"type":        p.Type,
				"description": p.Description,
			}
			required = append(required, p.Name)
		}

		tools = append(tools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": props,
					"required":   required,
				},
			},
		})
	}
	return tools
}

// ToJSON returns the tool definitions as JSON for debugging.
func (r *Registry) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r.List(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

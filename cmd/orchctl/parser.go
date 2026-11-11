package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	pb "orchestra/api/proto/orchestra/v1"
)

// ParseWorkflowFile loads a workflow spec from either a JSON or YAML file.
// Pure standard library implementation without external third-party YAML parser dependencies.
func ParseWorkflowFile(path string) (*pb.WorkflowSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("workflow file %s is empty", path)
	}

	// 1. If JSON, parse directly with standard library
	if trimmed[0] == '{' {
		var spec pb.WorkflowSpec
		if err := json.Unmarshal(trimmed, &spec); err != nil {
			return nil, fmt.Errorf("failed to parse JSON workflow spec: %w", err)
		}
		return &spec, nil
	}

	// 2. Parse YAML workflow spec
	return parseWorkflowYAML(string(trimmed))
}

func parseWorkflowYAML(content string) (*pb.WorkflowSpec, error) {
	spec := &pb.WorkflowSpec{
		FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
		Tasks:         make([]*pb.TaskSpec, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentTask *pb.TaskSpec
	inTasksSection := false
	inDependenciesSection := false

	for scanner.Scan() {
		rawLine := scanner.Text()
		trimmedLine := strings.TrimSpace(rawLine)

		// Ignore empty lines and comments
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		// Top-level / non-task keys
		if !inTasksSection {
			if strings.HasPrefix(trimmedLine, "tasks:") {
				inTasksSection = true
				continue
			}

			k, v, ok := splitKeyValue(trimmedLine)
			if ok {
				switch strings.ToLower(k) {
				case "id":
					spec.Id = unquote(v)
				case "name":
					spec.Name = unquote(v)
				case "failure_policy":
					policyVal := strings.ToUpper(unquote(v))
					if strings.Contains(policyVal, "CONTINUE") {
						spec.FailurePolicy = pb.FailurePolicy_FAILURE_POLICY_CONTINUE
					} else {
						spec.FailurePolicy = pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST
					}
				}
			}
			continue
		}

		// Inside tasks: section
		// Check for task start: "- id:"
		if strings.HasPrefix(trimmedLine, "- id:") || strings.HasPrefix(trimmedLine, "-") && strings.Contains(trimmedLine, "id:") {
			if currentTask != nil {
				spec.Tasks = append(spec.Tasks, currentTask)
			}
			currentTask = &pb.TaskSpec{
				Dependencies: make([]string, 0),
			}
			inDependenciesSection = false

			parts := strings.SplitN(trimmedLine, "id:", 2)
			if len(parts) == 2 {
				currentTask.Id = unquote(parts[1])
			}
			continue
		}

		if currentTask == nil {
			continue
		}

		// Check for dependencies list items: "- depName"
		if inDependenciesSection {
			if strings.HasPrefix(trimmedLine, "-") {
				dep := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "-"))
				if dep != "" {
					currentTask.Dependencies = append(currentTask.Dependencies, unquote(dep))
				}
				continue
			}
			inDependenciesSection = false
		}

		if strings.HasPrefix(trimmedLine, "dependencies:") {
			inDependenciesSection = true
			continue
		}

		// Task property
		k, v, ok := splitKeyValue(trimmedLine)
		if ok {
			switch strings.ToLower(k) {
			case "id":
				currentTask.Id = unquote(v)
			case "type":
				currentTask.Type = unquote(v)
			case "payload":
				currentTask.Payload = unquote(v)
			case "timeout_seconds":
				if sec, err := strconv.Atoi(unquote(v)); err == nil {
					currentTask.TimeoutSeconds = int64(sec)
				}
			case "max_retries":
				if retries, err := strconv.Atoi(unquote(v)); err == nil {
					currentTask.MaxRetries = int32(retries)
				}
			}
		}
	}

	if currentTask != nil {
		spec.Tasks = append(spec.Tasks, currentTask)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading YAML: %w", err)
	}

	if spec.Id == "" {
		return nil, fmt.Errorf("workflow missing required 'id' field")
	}
	if len(spec.Tasks) == 0 {
		return nil, fmt.Errorf("workflow must contain at least one task")
	}

	return spec, nil
}

func splitKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx == -1 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	return k, v, true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		if len(s) >= 2 {
			return s[1 : len(s)-1]
		}
	}
	return s
}

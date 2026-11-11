package main

import (
	"path/filepath"
	"testing"

	pb "orchestra/api/proto/orchestra/v1"
)

func TestParseWorkflowFile_DiamondYaml(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "diamond.yaml")
	spec, err := ParseWorkflowFile(path)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", path, err)
	}

	if spec.Id != "diamond-pipeline" {
		t.Errorf("expected id diamond-pipeline, got %s", spec.Id)
	}
	if spec.FailurePolicy != pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST {
		t.Errorf("expected FAIL_FAST policy, got %v", spec.FailurePolicy)
	}
	if len(spec.Tasks) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(spec.Tasks))
	}

	taskMap := make(map[string]*pb.TaskSpec)
	for _, task := range spec.Tasks {
		taskMap[task.Id] = task
	}

	// Verify diamond topology
	if len(taskMap["fetch"].Dependencies) != 0 {
		t.Errorf("fetch should have 0 dependencies")
	}
	if len(taskMap["transformA"].Dependencies) != 1 || taskMap["transformA"].Dependencies[0] != "fetch" {
		t.Errorf("transformA should depend on fetch")
	}
	if len(taskMap["transformB"].Dependencies) != 1 || taskMap["transformB"].Dependencies[0] != "fetch" {
		t.Errorf("transformB should depend on fetch")
	}
	if len(taskMap["merge"].Dependencies) != 2 {
		t.Errorf("merge should depend on transformA and transformB, got %v", taskMap["merge"].Dependencies)
	}
	if len(taskMap["publish"].Dependencies) != 1 || taskMap["publish"].Dependencies[0] != "merge" {
		t.Errorf("publish should depend on merge")
	}
}

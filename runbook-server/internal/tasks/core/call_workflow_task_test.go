package core

import (
	"context"
	"testing"

	"nudgebee/runbook/internal/model"
	"nudgebee/runbook/internal/tasks/testutils"

	"github.com/stretchr/testify/assert"
)

// liveVersionStore returns a draft definition via FindByName and a distinct LIVE
// version via GetLiveWorkflowVersion, so a test can prove which one is used.
type liveVersionStore struct {
	*testutils.MockWorkflowStore
	draftDef model.WorkflowDefinition
	liveDef  model.WorkflowDefinition
}

func (s *liveVersionStore) FindByName(ctx context.Context, tenantID, accountID, name string) (*model.Workflow, error) {
	return &model.Workflow{ID: "child-id", Name: name, TenantID: tenantID, AccountID: accountID, Definition: s.draftDef}, nil
}

func (s *liveVersionStore) GetLiveWorkflowVersion(ctx context.Context, workflowID string) (*model.WorkflowVersion, error) {
	return &model.WorkflowVersion{ID: "child-live-v", WorkflowID: workflowID, VersionNumber: 2, IsLive: true, Definition: s.liveDef}, nil
}

// TestCallWorkflowUsesLiveVersion is the H2 regression guard: core.call-workflow
// must build the child from the callee's LIVE published version, not its draft
// (workflows.definition). A published parent calling an edited-but-unpublished
// child must still run the child's last published graph.
func TestCallWorkflowUsesLiveVersion(t *testing.T) {
	store := &liveVersionStore{
		MockWorkflowStore: &testutils.MockWorkflowStore{},
		draftDef: model.WorkflowDefinition{
			Tasks: []model.Task{{ID: "draft-task", Type: "scripting.run_script"}},
		},
		liveDef: model.WorkflowDefinition{
			Tasks: []model.Task{{ID: "live-task", Type: "scripting.run_script"}},
		},
	}

	ctx := newTestContext().(*testutils.MockTaskContext)
	ctx.WfStore = store

	task := &CallWorkflowTask{}
	wfDef, err := task.GetChildWorkflowDefinition(ctx, map[string]any{"workflow_name": "child"})
	assert.NoError(t, err)
	assert.NotNil(t, wfDef)
	assert.Len(t, wfDef.Tasks, 1)
	assert.Equal(t, "live-task", wfDef.Tasks[0].ID, "child must run the callee's LIVE version, not its draft")
}

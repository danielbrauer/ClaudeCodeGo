package tui

import (
	"testing"
)

func TestE2E_DiffCommand_SwitchesToDiffMode(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/diff")

	if result.mode != modeDiff {
		t.Errorf("mode = %d, want modeDiff (%d)", result.mode, modeDiff)
	}
}

func TestE2E_DiffCommand_ClearsPreviousDiffData(t *testing.T) {
	m, _ := testModel(t)

	// Set some previous diff data.
	m.diffData = &diffData{
		errorMsg: "old error",
	}

	result, _ := submitCommand(m, "/diff")

	// diffData should be cleared while loading new data.
	if result.diffData != nil {
		t.Error("diffData should be nil while loading")
	}
}

func TestE2E_DiffCommand_DiffLoadedMsg(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeDiff

	// Simulate receiving loaded diff data.
	data := diffData{
		stats: diffStats{
			filesCount:   2,
			linesAdded:   10,
			linesRemoved: 3,
		},
		files: []diffFile{
			{path: "a.go", linesAdded: 5, linesRemoved: 1},
			{path: "b.go", linesAdded: 5, linesRemoved: 2},
		},
	}

	updated, _ := m.Update(DiffLoadedMsg{Data: data})
	result := updated.(model)

	if result.diffData == nil {
		t.Fatal("diffData should be set after DiffLoadedMsg")
	}
	if result.diffData.stats.filesCount != 2 {
		t.Errorf("filesCount = %d, want 2", result.diffData.stats.filesCount)
	}
	if result.diffSelected != 0 {
		t.Errorf("diffSelected = %d, want 0", result.diffSelected)
	}
	if result.diffViewMode != "list" {
		t.Errorf("diffViewMode = %q, want list", result.diffViewMode)
	}
}

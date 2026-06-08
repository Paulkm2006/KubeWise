package model_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	deploytypes "github.com/kubewise/kubewise/internal/agent/subagent/deploy/types"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/catalog"
	"github.com/kubewise/kubewise/internal/tui/model"
)

func TestNewDeployConfirmModelWithSize(t *testing.T) {
	plan := deploytypes.DeployPlan{
		DefaultValues: "default: values",
		CustomValues:  "custom: values",
	}
	m := model.NewDeployConfirmModel("q-1", plan, 120, 40)

	// 左右面板各占一半宽度
	expectedPanelW := (120 - 4) / 2 // 58
	expectedPanelH := 40 - 8        // 32

	if m.DefaultVPWidth() != expectedPanelW {
		t.Errorf("defaultVP width = %d, want %d", m.DefaultVPWidth(), expectedPanelW)
	}
	if m.DefaultVPHeight() != expectedPanelH {
		t.Errorf("defaultVP height = %d, want %d", m.DefaultVPHeight(), expectedPanelH)
	}
	if m.OverrideVPWidth() != expectedPanelW {
		t.Errorf("overrideVP width = %d, want %d", m.OverrideVPWidth(), expectedPanelW)
	}
	if m.OverrideVPHeight() != expectedPanelH {
		t.Errorf("overrideVP height = %d, want %d", m.OverrideVPHeight(), expectedPanelH)
	}
	// textarea.Width() 返回内部宽度（比 SetWidth 传入值小 6）
	if m.YAMLEditorWidth() != expectedPanelW-6 {
		t.Errorf("yamlEditor width = %d, want %d", m.YAMLEditorWidth(), expectedPanelW-6)
	}
	if m.YAMLEditorHeight() != expectedPanelH {
		t.Errorf("yamlEditor height = %d, want %d", m.YAMLEditorHeight(), expectedPanelH)
	}

	// 完整预览宽度 = 120-4, 高度 = 40-6
	expectedFullW := 120 - 4 // 116
	expectedFullH := 40 - 6  // 34

	if m.FullPreviewVPWidth() != expectedFullW {
		t.Errorf("fullPreviewVP width = %d, want %d", m.FullPreviewVPWidth(), expectedFullW)
	}
	if m.FullPreviewVPHeight() != expectedFullH {
		t.Errorf("fullPreviewVP height = %d, want %d", m.FullPreviewVPHeight(), expectedFullH)
	}
}

func TestNewDeployConfirmModelWithDifferentSize(t *testing.T) {
	plan := deploytypes.DeployPlan{
		DefaultValues: "default: values",
		CustomValues:  "custom: values",
	}
	m := model.NewDeployConfirmModel("q-1", plan, 80, 24)

	expectedPanelW := (80 - 4) / 2 // 38
	expectedPanelH := 24 - 8       // 16

	if m.DefaultVPWidth() != expectedPanelW {
		t.Errorf("defaultVP width = %d, want %d", m.DefaultVPWidth(), expectedPanelW)
	}
	if m.DefaultVPHeight() != expectedPanelH {
		t.Errorf("defaultVP height = %d, want %d", m.DefaultVPHeight(), expectedPanelH)
	}

	expectedFullW := 80 - 4 // 76
	expectedFullH := 24 - 6 // 18

	if m.FullPreviewVPWidth() != expectedFullW {
		t.Errorf("fullPreviewVP width = %d, want %d", m.FullPreviewVPWidth(), expectedFullW)
	}
	if m.FullPreviewVPHeight() != expectedFullH {
		t.Errorf("fullPreviewVP height = %d, want %d", m.FullPreviewVPHeight(), expectedFullH)
	}
}

func TestDeployConfirmFocusPanel(t *testing.T) {
	plan := deploytypes.DeployPlan{
		ChartInfo: &catalog.ChartInfo{
			RepoName:         "testrepo",
			ChartName:        "testchart",
			Source:           "test",
			DefaultNamespace: "default",
		},
		DefaultValues: "default: val",
		CustomValues:  "custom: val",
		ReleaseName:   "test",
		Namespace:     "default",
	}
	m := model.NewDeployConfirmModel("q-1", plan, 120, 40)

	// Initial focus should be left panel (0)
	if m.FocusPanel() != 0 {
		t.Errorf("expected initial focusPanel=0, got %d", m.FocusPanel())
	}

	// Simulate Tab press
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.FocusPanel() != 1 {
		t.Errorf("expected focusPanel=1 after Tab, got %d", updated.FocusPanel())
	}

	// Simulate Tab again — should go back to 0
	updated2, _ := updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated2.FocusPanel() != 0 {
		t.Errorf("expected focusPanel=0 after second Tab, got %d", updated2.FocusPanel())
	}

	// Verify View() does not crash (no nil pointer dereference)
	_ = updated2.View()
}

func TestDeployConfirmViewScrollKeys(t *testing.T) {
	// 生成足够多的行，超过 viewport 高度 (32)
	tallContent := ""
	for i := 0; i < 50; i++ {
		tallContent += fmt.Sprintf("key_%d: value_%d\n", i, i)
	}
	plan := deploytypes.DeployPlan{
		ChartInfo:     &catalog.ChartInfo{RepoName: "r", ChartName: "c", Source: "s", DefaultNamespace: "ns"},
		DefaultValues: tallContent,
		CustomValues:  tallContent,
		ReleaseName:   "test",
		Namespace:     "ns",
	}
	m := model.NewDeployConfirmModel("q-1", plan, 120, 40)

	// 滚动前记录 left panel view
	viewBefore := m.View()

	// 发送 down 键 — 应该滚动焦点面板 (focusPanel=0, left/defaultVP)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewAfter := updated.View()

	if viewBefore == viewAfter {
		t.Error("expected left viewport to scroll on down key, but view didn't change")
	}

	// 发送 up 键 — 应该滚回去
	m2 := model.NewDeployConfirmModel("q-1", plan, 120, 40)
	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewScrolled := m2.View()
	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyUp})
	viewScrolledUp := m2.View()
	if viewScrolled == viewScrolledUp {
		t.Error("expected left viewport to scroll up on up key, but view didn't change")
	}

	// Tab 切换到右面板后，down 键应该滚动 overrideVP
	viewLeftBefore := m2.View()
	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab}) // focusPanel = 1
	viewAfterTab := m2.View()
	// Tab 只切换焦点边框，不改变 viewport 内容位置，view 应不同于初始但 scroll 状态一致
	_ = viewAfterTab

	m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewRightScrolled := m2.View()
	// 右面板滚动后整体 view 应与只滚动左面板时不同
	if viewLeftBefore == viewRightScrolled {
		t.Error("expected right viewport to scroll after Tab + down, but view didn't change")
	}
}

func TestDeployConfirmViewMouseWheel(t *testing.T) {
	tallContent := ""
	for i := 0; i < 50; i++ {
		tallContent += fmt.Sprintf("key_%d: value_%d\n", i, i)
	}
	plan := deploytypes.DeployPlan{
		ChartInfo:     &catalog.ChartInfo{RepoName: "r", ChartName: "c", Source: "s", DefaultNamespace: "ns"},
		DefaultValues: tallContent,
		CustomValues:  tallContent,
		ReleaseName:   "test",
		Namespace:     "ns",
	}
	m := model.NewDeployConfirmModel("q-1", plan, 120, 40)

	viewBefore := m.View()
	updated, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	viewAfter := updated.View()
	if viewBefore == viewAfter {
		t.Error("expected viewport to scroll on mouse wheel down, but view didn't change")
	}

	// Wheel up should scroll back
	updated, _ = updated.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	viewAfterUp := updated.View()
	if viewAfter == viewAfterUp {
		t.Error("expected viewport to scroll on mouse wheel up, but view didn't change")
	}
}

func TestDeployConfirmFullPreviewMouseWheel(t *testing.T) {
	tallContent := ""
	for i := 0; i < 80; i++ {
		tallContent += fmt.Sprintf("key_%d: value_%d\n", i, i)
	}
	plan := deploytypes.DeployPlan{
		ChartInfo:     &catalog.ChartInfo{RepoName: "r", ChartName: "c", Source: "s", DefaultNamespace: "ns"},
		DefaultValues: tallContent,
		CustomValues:  "custom: val",
		ReleaseName:   "test",
		Namespace:     "ns",
	}
	m := model.NewDeployConfirmModel("q-1", plan, 120, 40)

	// 按 V 进入 full preview 模式
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})

	viewBefore := updated.View()
	updated, _ = updated.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	viewAfter := updated.View()
	if viewBefore == viewAfter {
		t.Error("expected full preview to scroll on mouse wheel down, but view didn't change")
	}
}

func TestDeployConfirm_RendersWarnings(t *testing.T) {
	plan := deploytypes.DeployPlan{
		ChartInfo:     &catalog.ChartInfo{RepoName: "r", ChartName: "c", Source: "s", DefaultNamespace: "ns"},
		DefaultValues: "replicas: 1",
		CustomValues:  "privileged: true",
		ReleaseName:   "test",
		Namespace:     "ns",
		Warnings: []deploytypes.PlanWarning{
			{Severity: "warn", Message: "hostNetwork: true"},
		},
	}
	m := model.NewDeployConfirmModel("q-1", plan, 120, 40)
	view := m.View()
	if !strings.Contains(view, "策略与校验提示") || !strings.Contains(view, "hostNetwork") {
		t.Fatalf("expected warnings in view, got:\n%s", view)
	}
}

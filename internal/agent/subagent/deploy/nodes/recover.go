package nodes

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/plan"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/recovery"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/state"
	"github.com/kubewise/kubewise/internal/agent/event"
)

type stateRecoveryLogger struct{ st *state.State }

func (l *stateRecoveryLogger) Info(msg string, fields ...zap.Field)  { l.st.LogInfo(msg, fields...) }
func (l *stateRecoveryLogger) Debug(msg string, fields ...zap.Field) { l.st.LogDebug(msg, fields...) }
func (l *stateRecoveryLogger) Warn(msg string, fields ...zap.Field)  { l.st.LogWarn(msg, fields...) }
func (l *stateRecoveryLogger) Error(msg string, fields ...zap.Field) { l.st.LogError(msg, fields...) }

// RecoverDeployment runs the post-failure ReAct recovery loop.
func RecoverDeployment(st *state.State) error {
	st.Emit(event.Phase{QueryID: st.QueryID, Phase: "诊断修复中"})
	st.LogInfo("entering recovery loop", zap.String("app", st.AppName))
	if st.RecoveryAttempts >= st.MaxRecoveryAttempts {
		st.LogWarn("recovery retry limit reached",
			zap.Int("attempts", st.RecoveryAttempts),
			zap.Int("max", st.MaxRecoveryAttempts),
		)
		st.Done(fmt.Sprintf("已达到最大重新部署次数（%d次），诊断修复已停止。请手动检查集群状态。", st.MaxRecoveryAttempts))
		return nil
	}

	runner := &recovery.Runner{
		QueryID: st.QueryID,
		LLM:     st.LLM,
		Helm:    st.Helm,
		Tools:   st.Tools,
		K8s:     st.K8s,
		Log:     &stateRecoveryLogger{st: st},
	}
	result, recErr := runner.Run(st.Ctx, recovery.RunInput{
		FailureErr:        recoveryError(st),
		Query:             st.Query,
		CorrectionHistory: st.CorrectionHistory,
		Chart:             st.Chart,
		DefaultValues:     st.DefaultValues,
		CurrentValues:     st.FinalValues,
		TargetNS:          st.Plan.Namespace,
		AppName:           st.AppName,
		Messages:          st.RecoveryMessages,
	})
	if recErr != nil {
		st.LogError("recovery loop error", zap.Error(recErr))
		return fmt.Errorf("诊断修复过程出错: %w", recErr)
	}
	st.RecoveryMessages = result.Messages
	st.LogInfo("recovery loop finished",
		zap.Int("action", int(result.Action)),
		zap.String("reason", result.Reason),
	)
	if result.Action == recovery.ActionRecovered {
		return applyRecoveredValues(st, result)
	}
	st.Done(result.Details)
	return nil
}

func applyRecoveredValues(st *state.State, result *recovery.Result) error {
	if result == nil || result.YAML == "" {
		return fmt.Errorf("诊断修复未返回可部署的 values")
	}
	st.RecoveryAttempts++
	st.FinalValues = plan.ApplyCRDValues(st.Chart, st.DefaultValues, result.YAML)
	st.Plan.CustomValues = st.FinalValues
	st.Plan.DefaultValues = st.DefaultValues
	st.Plan.Chart = st.Chart
	st.Plan.AppName = st.AppName
	if st.Plan.ReleaseName == "" {
		st.Plan.ReleaseName = st.ReleaseName
	}
	if st.Plan.Namespace == "" {
		st.Plan.Namespace = "default"
	}
	st.Plan.Warnings = nil
	if err := validatePlan(st, "recovery"); err != nil {
		return err
	}
	st.RecoveryPendingReview = true
	st.EmitCritical(event.LLMTextDelta{QueryID: st.QueryID, Delta: "最终 Values:\n" + st.FinalValues})
	if result.Summary != "" {
		st.EmitCritical(event.LLMTextDelta{QueryID: st.QueryID, Delta: result.Summary})
	}
	st.LogInfo("recovery values accepted by state",
		zap.Int("attempt", st.RecoveryAttempts),
		zap.Int("max", st.MaxRecoveryAttempts),
		zap.Int("values_lines", state.CountLines(st.FinalValues)),
	)
	st.Next(state.PhasePreflight)
	return nil
}

func recoveryError(st *state.State) error {
	if st.RecoveryErr != nil {
		return st.RecoveryErr
	}
	return st.DeployErr
}

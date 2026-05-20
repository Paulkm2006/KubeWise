package deploy

import (
	"strings"

	"go.uber.org/zap"
)

func (a *Agent) baseFields() []zap.Field {
	fields := []zap.Field{zap.String("component", "deploy")}
	if a.queryID != "" {
		fields = append(fields, zap.String("query_id", a.queryID))
	}
	return fields
}

func (a *Agent) logDebug(msg string, fields ...zap.Field) {
	a.logger().Debug(msg, append(a.baseFields(), fields...)...)
}

func (a *Agent) logInfo(msg string, fields ...zap.Field) {
	a.logger().Info(msg, append(a.baseFields(), fields...)...)
}

func (a *Agent) logWarn(msg string, fields ...zap.Field) {
	a.logger().Warn(msg, append(a.baseFields(), fields...)...)
}

func (a *Agent) logError(msg string, fields ...zap.Field) {
	a.logger().Error(msg, append(a.baseFields(), fields...)...)
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

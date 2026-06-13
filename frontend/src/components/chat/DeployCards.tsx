import { useState } from 'react';
import type { ChartCandidate, DeployPlan } from '../../api/types';

interface ChartSelectCardProps {
  appName?: string;
  candidates: ChartCandidate[];
  disabled?: boolean;
  onSelect: (index: number) => void;
  onManual: (repoUrl: string, chartName: string) => void;
  onCancel: () => void;
}

export function ChartSelectCard({
  appName,
  candidates,
  disabled,
  onSelect,
  onManual,
  onCancel,
}: ChartSelectCardProps) {
  const [manual, setManual] = useState(false);
  const [repoUrl, setRepoUrl] = useState('');
  const [chartName, setChartName] = useState('');

  return (
    <div className="mt-3 p-4 rounded-sm border border-accent/25 bg-accent-dim/10 space-y-3">
      <div className="text-xs font-mono text-accent uppercase tracking-wide">选择 Helm Chart</div>
      {appName && <div className="text-sm text-text-secondary">应用: {appName}</div>}

      {!manual ? (
        <>
          <div className="space-y-2 max-h-48 overflow-y-auto">
            {candidates.map((c, i) => (
              <button
                key={`${c.ChartName}-${i}`}
                type="button"
                disabled={disabled}
                onClick={() => onSelect(i)}
                className="w-full text-left p-2 rounded-sm border border-border hover:border-accent/40 bg-bg disabled:opacity-40"
              >
                <div className="text-sm font-medium text-text">
                  {c.ChartName}
                  {c.LatestVersion ? ` @ ${c.LatestVersion}` : ''}
                </div>
                <div className="text-xs text-text-muted font-mono truncate">{c.RepoURL}</div>
                {c.Description && <div className="text-xs text-text-secondary mt-1">{c.Description}</div>}
              </button>
            ))}
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={disabled}
              onClick={() => setManual(true)}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-secondary"
            >
              手动输入 Chart
            </button>
            <button
              type="button"
              disabled={disabled}
              onClick={onCancel}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-muted"
            >
              取消
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-2">
          <input
            value={repoUrl}
            onChange={(e) => setRepoUrl(e.target.value)}
            placeholder="Repo URL"
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2"
          />
          <input
            value={chartName}
            onChange={(e) => setChartName(e.target.value)}
            placeholder="Chart name"
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={disabled || !repoUrl || !chartName}
              onClick={() => onManual(repoUrl.trim(), chartName.trim())}
              className="text-sm px-3 py-1.5 rounded-sm bg-accent text-bg font-medium disabled:opacity-40"
            >
              使用此 Chart
            </button>
            <button type="button" onClick={() => setManual(false)} className="text-sm px-3 py-1.5 rounded-sm border border-border">
              返回
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

type ValuesTab = 'override' | 'defaults';

interface DeployConfirmCardProps {
  plan: DeployPlan;
  disabled?: boolean;
  onExecute: (values: string) => void;
  onCancel: () => void;
  onCorrect: (values: string, correction: string) => void;
}

export function DeployConfirmCard({ plan, disabled, onExecute, onCancel, onCorrect }: DeployConfirmCardProps) {
  const [values, setValues] = useState(plan.CustomValues || '');
  const [correction, setCorrection] = useState('');
  const [tab, setTab] = useState<ValuesTab>('override');

  const defaults = plan.DefaultValues || '';
  const chartName = plan.ChartInfo?.ChartName || 'chart';
  const release = plan.ReleaseName || 'release';
  const namespace = plan.Namespace || 'default';

  return (
    <div className="mt-3 w-full min-w-0 p-4 rounded-sm border border-accent/30 bg-accent-dim/10 space-y-4">
      <div className="space-y-1">
        <div className="text-xs font-mono text-accent uppercase tracking-wide">部署预览</div>
        <div className="text-sm text-text flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="font-medium">{chartName}</span>
          <span className="text-text-muted">→</span>
          <span className="font-mono text-text-secondary">{namespace}/{release}</span>
          {plan.IsUpgrade && (
            <span className="text-xs px-1.5 py-0.5 rounded-sm bg-amber-dim/40 text-amber border border-amber/20">
              升级
            </span>
          )}
        </div>
      </div>

      {plan.Warnings && plan.Warnings.length > 0 && (
        <div className="space-y-1">
          {plan.Warnings.map((w, i) => (
            <div key={i} className="text-xs text-amber bg-amber-dim/30 px-2 py-1.5 rounded-sm border border-amber/15">
              {w.Message}
            </div>
          ))}
        </div>
      )}

      <div className="space-y-2">
        <div className="flex gap-1 border-b border-border">
          <button
            type="button"
            onClick={() => setTab('override')}
            className={`text-sm px-3 py-2 -mb-px border-b-2 transition-colors ${
              tab === 'override'
                ? 'border-accent text-accent'
                : 'border-transparent text-text-muted hover:text-text-secondary'
            }`}
          >
            覆盖值
            <span className="ml-1.5 text-xs text-text-muted">（可编辑）</span>
          </button>
          <button
            type="button"
            onClick={() => setTab('defaults')}
            className={`text-sm px-3 py-2 -mb-px border-b-2 transition-colors ${
              tab === 'defaults'
                ? 'border-accent text-accent'
                : 'border-transparent text-text-muted hover:text-text-secondary'
            }`}
          >
            Chart 默认值
            <span className="ml-1.5 text-xs text-text-muted">（只读）</span>
          </button>
        </div>

        {tab === 'override' ? (
          <textarea
            value={values}
            onChange={(e) => setValues(e.target.value)}
            rows={14}
            spellCheck={false}
            disabled={disabled}
            className="w-full text-xs font-mono leading-relaxed bg-bg border border-border rounded-sm p-3 text-text-secondary
                       outline-none focus:border-accent/40 disabled:opacity-50 resize-y min-h-[200px]"
            placeholder="# Helm 覆盖值 (YAML)"
          />
        ) : (
          <pre
            className="w-full text-xs font-mono leading-relaxed bg-bg border border-border rounded-sm p-3 text-text-muted
                       overflow-auto max-h-80 min-h-[200px] whitespace-pre-wrap"
          >
            {defaults || '(no default values)'}
          </pre>
        )}
      </div>

      <div className="space-y-2 pt-1 border-t border-border/60">
        <div className="text-sm font-medium text-text">自然语言修正</div>
        <p className="text-xs text-text-muted">
          描述需要修改的内容 — Agent 将重新生成覆盖值并展示更新后的部署计划。
        </p>
        <textarea
          value={correction}
          onChange={(e) => setCorrection(e.target.value)}
          rows={2}
          disabled={disabled}
          placeholder="例如：设置 NodePort 为 30090，副本数增加到 3"
          className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2 text-text
                     outline-none focus:border-accent/40 disabled:opacity-50 resize-y"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && correction.trim()) {
              e.preventDefault();
              onCorrect(values, correction.trim());
              setCorrection('');
            }
          }}
        />
        <button
          type="button"
          disabled={disabled || !correction.trim()}
          onClick={() => {
            onCorrect(values, correction.trim());
            setCorrection('');
          }}
          className="text-sm px-4 py-2 rounded-sm bg-accent text-bg font-medium disabled:opacity-40"
        >
          重新生成值
        </button>
      </div>

      <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-border/60">
        <button
          type="button"
          disabled={disabled}
          onClick={() => onExecute(values)}
          className="text-sm px-4 py-2 rounded-sm bg-green text-bg font-medium disabled:opacity-40"
        >
          使用此配置部署
        </button>
        <button
          type="button"
          disabled={disabled}
          onClick={onCancel}
          className="text-sm px-4 py-2 rounded-sm border border-border text-text-muted hover:border-red/30 hover:text-red"
        >
          取消部署
        </button>
        <span className="text-xs text-text-muted ml-auto hidden sm:inline">
          提示：可直接编辑 YAML，或使用上方自然语言修正
        </span>
      </div>
    </div>
  );
}

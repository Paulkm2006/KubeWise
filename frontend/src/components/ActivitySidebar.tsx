import { Activity } from '../data/mock';

interface ActivitySidebarProps {
  activities: Activity[];
  activeCluster: string;
}

const typeStyle: Record<string, { border: string; dot: string }> = {
  done: { border: 'border-l-green/50', dot: 'bg-green' },
  issue: { border: 'border-l-red/50', dot: 'bg-red' },
  pending: { border: 'border-l-amber/50', dot: 'bg-amber' },
  info: { border: 'border-l-accent/50', dot: 'bg-accent' },
};

export default function ActivitySidebar({ activities, activeCluster }: ActivitySidebarProps) {
  return (
    <aside className="w-[180px] shrink-0 flex flex-col bg-bg">
      {/* Activity Feed */}
      <div className="flex-1 flex flex-col min-h-0 p-3">
        <div className="flex items-center justify-between mb-2.5">
          <span className="text-xxs text-text-muted tracking-widest uppercase font-medium">事件</span>
          <span className="text-xxs text-text-muted cursor-pointer hover:text-text-secondary transition-colors">
            清空
          </span>
        </div>
        <div className="flex-1 overflow-y-auto space-y-0.5">
          {activities.map((a, i) => {
            const s = typeStyle[a.type] || typeStyle.info;
            return (
              <div
                key={i}
                className={`flex items-start gap-2 px-2 py-1.5 border-l-2 ${s.border} hover:bg-hover/50 transition-colors cursor-default rounded-r-sm`}
              >
                <span className={`mt-[5px] w-1 h-1 rounded-full shrink-0 ${s.dot}`} />
                <div className="min-w-0 flex-1">
                  <p className="text-xxs text-text-secondary leading-tight truncate">{a.text}</p>
                  <p className="text-[10px] text-text-muted mt-px font-mono">
                    {a.cluster ? `${a.cluster} · ` : ''}{a.time}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Cluster State Card */}
      <div className="px-3 pb-3">
        <div className="p-2.5 bg-elevated border border-border rounded-sm">
          <p className="text-xs font-medium text-text">{activeCluster}</p>
          <div className="flex items-center gap-1.5 mt-1">
            <span className="text-xxs px-1.5 py-0.5 rounded-sm bg-amber-dim text-amber tracking-wider font-medium">
              2 个问题
            </span>
            <span className="text-xxs text-text-muted font-mono">8/10</span>
          </div>
          <div className="flex gap-2.5 mt-1.5 text-[10px] text-text-muted tracking-wide">
            <span>◈ 10 节点</span>
            <span>▣ 28 命名空间</span>
          </div>
        </div>
      </div>
    </aside>
  );
}

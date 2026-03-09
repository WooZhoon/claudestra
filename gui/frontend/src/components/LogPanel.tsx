import { useRef, useEffect, useState, memo } from 'react';

export interface LogEntry {
  type: 'text' | 'thinking' | 'status';
  message: string;
}

interface LogPanelProps {
  logs: LogEntry[];
}

export default memo(function LogPanel({ logs }: LogPanelProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const [thinkingCollapsed, setThinkingCollapsed] = useState(true);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    // 사용자가 스크롤을 올렸으면 자동 스크롤 하지 않음
    const isNearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 120;
    if (isNearBottom) {
      bottomRef.current?.scrollIntoView({ behavior: 'auto' });
    }
  }, [logs]);

  // 연속된 thinking 항목을 그룹화
  const grouped = groupLogs(logs);

  return (
    <div
      ref={containerRef}
      style={{
        flex: 1,
        background: 'var(--bg-primary)',
        overflowY: 'auto',
        padding: '12px 16px',
        fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
        fontSize: 13,
        lineHeight: 1.6,
      }}
    >
      {grouped.map((item, i) => {
        if (item.kind === 'thinking-group') {
          return (
            <ThinkingGroup
              key={i}
              entries={item.entries}
              collapsed={thinkingCollapsed}
              onToggle={() => setThinkingCollapsed(c => !c)}
            />
          );
        }
        const log = item.entry;
        return (
          <div key={i} style={{
            color: getLogColor(log.message),
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}>
            {log.message}
          </div>
        );
      })}
      <div ref={bottomRef} />
    </div>
  );
});

function ThinkingGroup({
  entries,
  collapsed,
  onToggle,
}: {
  entries: LogEntry[];
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <div style={{
      margin: '4px 0',
      borderLeft: '2px solid rgba(124, 140, 200, 0.3)',
      paddingLeft: 10,
    }}>
      <div
        onClick={onToggle}
        style={{
          cursor: 'pointer',
          color: 'var(--text-muted)',
          fontSize: 12,
          userSelect: 'none',
          display: 'flex',
          alignItems: 'center',
          gap: 4,
        }}
      >
        <span style={{
          display: 'inline-block',
          transform: collapsed ? 'rotate(0deg)' : 'rotate(90deg)',
          transition: 'transform 0.15s',
        }}>
          ▶
        </span>
        💭 사고 과정 ({entries.length}개 청크)
      </div>
      {!collapsed && (
        <div style={{
          marginTop: 4,
          padding: '6px 8px',
          background: 'rgba(124, 140, 200, 0.05)',
          borderRadius: 4,
          fontSize: 12,
          color: 'var(--text-muted)',
          fontStyle: 'italic',
          maxHeight: 200,
          overflowY: 'auto',
        }}>
          {entries.map((e, i) => (
            <span key={i} style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
              {e.message.replace(/^.*?💭\s*/, '')}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

type GroupedItem =
  | { kind: 'single'; entry: LogEntry }
  | { kind: 'thinking-group'; entries: LogEntry[] };

function groupLogs(logs: LogEntry[]): GroupedItem[] {
  const result: GroupedItem[] = [];
  let i = 0;
  while (i < logs.length) {
    if (logs[i].type === 'thinking') {
      const group: LogEntry[] = [];
      while (i < logs.length && logs[i].type === 'thinking') {
        group.push(logs[i]);
        i++;
      }
      result.push({ kind: 'thinking-group', entries: group });
    } else {
      result.push({ kind: 'single', entry: logs[i] });
      i++;
    }
  }
  return result;
}

function getLogColor(msg: string): string {
  if (msg.includes('❌') || msg.includes('오류')) return 'var(--error)';
  if (msg.includes('✅') || msg.includes('완료')) return 'var(--success)';
  if (msg.includes('📌') || msg.includes('📋')) return 'var(--accent)';
  if (msg.includes('⚠️')) return 'var(--warning)';
  return 'var(--text-secondary)';
}

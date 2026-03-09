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
  const [collapsedGroups, setCollapsedGroups] = useState<Record<number, boolean>>({});

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

  // thinking 그룹 인덱스 카운터
  let thinkingGroupIdx = 0;

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
          const gIdx = thinkingGroupIdx++;
          const collapsed = collapsedGroups[gIdx] ?? true;
          return (
            <ThinkingGroup
              key={i}
              entries={item.entries}
              collapsed={collapsed}
              onToggle={() => setCollapsedGroups(prev => ({ ...prev, [gIdx]: !collapsed }))}
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

// 🔧 이모지가 포함된 메시지인지 체크
function isToolUseEntry(entry: LogEntry): boolean {
  return entry.message.includes('🔧');
}

// 📊 분석 텍스트 항목인지 체크
function isAnalysisEntry(entry: LogEntry): boolean {
  return entry.message.includes('📊');
}

function ThinkingGroup({
  entries,
  collapsed,
  onToggle,
}: {
  entries: LogEntry[];
  collapsed: boolean;
  onToggle: () => void;
}) {
  const thinkingCount = entries.filter(e => !isToolUseEntry(e) && !isAnalysisEntry(e)).length;
  const toolCount = entries.filter(e => isToolUseEntry(e)).length;
  const analysisCount = entries.filter(e => isAnalysisEntry(e)).length;

  const parts: string[] = [];
  if (thinkingCount > 0) parts.push(`${thinkingCount}개 청크`);
  if (toolCount > 0) parts.push(`🔧 ${toolCount}개 도구 호출`);
  if (analysisCount > 0) parts.push(`📊 분석`);
  const label = `💭 사고 과정 (${parts.join(', ')})`;

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
        {label}
      </div>
      {!collapsed && (
        <div style={{
          marginTop: 4,
          padding: '6px 8px',
          background: 'rgba(124, 140, 200, 0.05)',
          borderRadius: 4,
          fontSize: 12,
          maxHeight: 200,
          overflowY: 'auto',
        }}>
          {entries.map((e, i) => {
            if (isToolUseEntry(e)) {
              // 도구 호출: 다른 스타일로 표시
              const toolMsg = e.message.replace(/^.*?🔧\s*/, '🔧 ');
              return (
                <div key={i} style={{
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  color: 'var(--accent)',
                  fontStyle: 'normal',
                  fontFamily: "'JetBrains Mono', monospace",
                  fontSize: 11,
                  padding: '2px 6px',
                  margin: '4px 0',
                  background: 'rgba(124, 140, 200, 0.08)',
                  borderRadius: 3,
                }}>
                  {toolMsg}
                </div>
              );
            }
            if (isAnalysisEntry(e)) {
              // 분석 텍스트: 일반 스타일로 표시
              return (
                <div key={i} style={{
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  color: 'var(--text-secondary)',
                  fontStyle: 'normal',
                  fontSize: 12,
                  lineHeight: 1.6,
                  padding: '4px 0',
                }}>
                  {e.message.replace(/^.*?📊\s*/, '')}
                </div>
              );
            }
            // thinking: 이탤릭
            return (
              <div key={i} style={{
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                color: 'var(--text-muted)',
                fontStyle: 'italic',
              }}>
                {e.message.replace(/^.*?💭\s*/, '')}
              </div>
            );
          })}
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
  if (msg.includes('❌')) return 'var(--error)';
  if (msg.includes('✅') || msg.includes('완료')) return 'var(--success)';
  if (msg.includes('📌') || msg.includes('📋')) return 'var(--accent)';
  if (msg.includes('⚠️')) return 'var(--warning)';
  return 'var(--text-secondary)';
}

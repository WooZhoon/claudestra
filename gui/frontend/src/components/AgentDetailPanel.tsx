interface AgentDetail {
  id: string;
  role: string;
  status: string;
  isConsumer: boolean;
  instruction: string;
  output: string;
  allowedTools: string[];
}

interface AgentDetailPanelProps {
  detail: AgentDetail | null;
  visible: boolean;
  onClose: () => void;
}

const statusLabel: Record<string, { text: string; color: string }> = {
  IDLE: { text: '대기 중', color: 'var(--text-muted)' },
  RUNNING: { text: '실행 중', color: 'var(--accent)' },
  DONE: { text: '완료', color: 'var(--success)' },
  ERROR: { text: '오류', color: 'var(--error)' },
};

export type { AgentDetail };

export default function AgentDetailPanel({ detail, visible, onClose }: AgentDetailPanelProps) {
  if (!visible || !detail) return null;

  const status = statusLabel[detail.status] || { text: detail.status, color: 'var(--text-muted)' };

  return (
    <div style={{
      position: 'absolute',
      top: 0,
      right: 0,
      width: '50%',
      height: '100%',
      background: 'var(--bg-secondary)',
      borderLeft: '1px solid var(--border)',
      display: 'flex',
      flexDirection: 'column',
      zIndex: 10,
    }}>
      {/* 헤더 */}
      <div style={{
        padding: '12px 16px',
        borderBottom: '1px solid var(--border)',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ fontWeight: 600, color: 'var(--text-primary)', fontSize: 15 }}>
            {detail.role}
          </span>
          <span style={{
            fontSize: 11,
            padding: '2px 8px',
            borderRadius: 4,
            background: 'var(--bg-primary)',
            color: status.color,
            fontWeight: 600,
          }}>
            {status.text}
          </span>
          <span style={{
            fontSize: 10,
            padding: '2px 6px',
            borderRadius: 4,
            background: detail.isConsumer ? 'var(--warning)' : 'var(--accent)',
            color: '#1a1b26',
            fontWeight: 600,
          }}>
            {detail.isConsumer ? 'Consumer' : 'Producer'}
          </span>
        </div>
        <button
          onClick={onClose}
          style={{
            background: 'none',
            border: 'none',
            color: 'var(--text-muted)',
            cursor: 'pointer',
            fontSize: 18,
          }}
        >
          ✕
        </button>
      </div>

      {/* 본문 */}
      <div style={{ flex: 1, overflowY: 'auto', padding: 16 }}>
        {/* 허용 도구 */}
        {detail.allowedTools && detail.allowedTools.length > 0 && (
          <Section title="허용 도구">
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              {detail.allowedTools.map(tool => (
                <span key={tool} style={{
                  fontSize: 11,
                  padding: '3px 8px',
                  borderRadius: 4,
                  background: 'var(--bg-tertiary)',
                  color: 'var(--text-secondary)',
                  border: '1px solid var(--border)',
                }}>
                  {tool}
                </span>
              ))}
            </div>
          </Section>
        )}

        {/* 받은 지시 */}
        <Section title="받은 지시 (Instruction)">
          {detail.instruction ? (
            <pre style={{
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              fontSize: 12,
              lineHeight: 1.7,
              color: 'var(--text-secondary)',
              background: 'var(--bg-primary)',
              padding: 12,
              borderRadius: 6,
              border: '1px solid var(--border)',
              maxHeight: 200,
              overflowY: 'auto',
            }}>
              {detail.instruction}
            </pre>
          ) : (
            <Empty text="아직 지시가 없습니다" />
          )}
        </Section>

        {/* 실행 결과 */}
        <Section title="실행 결과 (Output)">
          {detail.output ? (
            <pre style={{
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              fontSize: 12,
              lineHeight: 1.7,
              color: 'var(--text-secondary)',
              background: 'var(--bg-primary)',
              padding: 12,
              borderRadius: 6,
              border: '1px solid var(--border)',
              maxHeight: 400,
              overflowY: 'auto',
            }}>
              {detail.output}
            </pre>
          ) : (
            <Empty text="아직 결과가 없습니다" />
          )}
        </Section>
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: 20 }}>
      <div style={{
        fontSize: 12,
        color: 'var(--text-muted)',
        marginBottom: 8,
        fontWeight: 600,
        textTransform: 'uppercase',
        letterSpacing: 0.5,
      }}>
        {title}
      </div>
      {children}
    </div>
  );
}

function Empty({ text }: { text: string }) {
  return (
    <div style={{
      fontSize: 12,
      color: 'var(--text-muted)',
      padding: '12px',
      background: 'var(--bg-primary)',
      borderRadius: 6,
      border: '1px solid var(--border)',
      textAlign: 'center',
    }}>
      {text}
    </div>
  );
}

interface TeamPlan {
  role: string;
  description: string;
  type: string;
}

interface Task {
  agentId: string;
  instruction: string;
}

interface Step {
  step: number;
  title: string;
  tasks: Task[];
}

interface Proposal {
  userInput: string;
  teamPlans: TeamPlan[] | null;
  steps: Step[];
  contract: string;
  needTeam: boolean;
}

interface ProposalPanelProps {
  proposal: Proposal | null;
  visible: boolean;
  onExecute: () => void;
  onCancel: () => void;
  executing: boolean;
}

export type { Proposal };

export default function ProposalPanel({ proposal, visible, onExecute, onCancel, executing }: ProposalPanelProps) {
  if (!visible || !proposal) return null;

  return (
    <div style={{
      position: 'absolute',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      background: 'rgba(0,0,0,0.5)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 20,
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        borderRadius: 12,
        border: '1px solid var(--border)',
        width: '80%',
        maxWidth: 700,
        maxHeight: '80%',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}>
        {/* 헤더 */}
        <div style={{
          padding: '16px 20px',
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}>
          <div>
            <div style={{ fontWeight: 600, fontSize: 16, color: 'var(--accent)' }}>
              실행 계획 검토
            </div>
            <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 4 }}>
              "{proposal.userInput}"
            </div>
          </div>
          <button
            onClick={onCancel}
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
        <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
          {/* 팀 구성 */}
          {proposal.needTeam && proposal.teamPlans && proposal.teamPlans.length > 0 && (
            <Section title="팀 구성">
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {proposal.teamPlans.map(plan => (
                  <div key={plan.role} style={{
                    padding: '10px 14px',
                    borderRadius: 8,
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border)',
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                      <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-primary)' }}>
                        {plan.role}
                      </span>
                      <span style={{
                        fontSize: 10,
                        padding: '2px 6px',
                        borderRadius: 4,
                        background: plan.type === 'producer' ? 'var(--accent)' : 'var(--warning)',
                        color: '#1a1b26',
                        fontWeight: 600,
                      }}>
                        {plan.type}
                      </span>
                    </div>
                    <div style={{ fontSize: 12, color: 'var(--text-muted)', whiteSpace: 'pre-wrap', lineHeight: 1.5 }}>
                      {plan.description.length > 150 ? plan.description.slice(0, 150) + '...' : plan.description}
                    </div>
                  </div>
                ))}
              </div>
            </Section>
          )}

          {/* 실행 계획 */}
          <Section title="실행 계획">
            {proposal.steps.map(step => (
              <div key={step.step} style={{ marginBottom: 12 }}>
                <div style={{
                  fontSize: 13,
                  fontWeight: 600,
                  color: 'var(--text-primary)',
                  marginBottom: 6,
                }}>
                  {step.step}단계: {step.title}
                </div>
                {step.tasks.map((task, i) => (
                  <div key={i} style={{
                    padding: '8px 12px',
                    marginLeft: 16,
                    marginBottom: 4,
                    borderRadius: 6,
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border)',
                    fontSize: 12,
                  }}>
                    <span style={{ color: 'var(--accent)', fontWeight: 600 }}>[{task.agentId}]</span>
                    <span style={{ color: 'var(--text-secondary)', marginLeft: 8 }}>
                      {task.instruction.length > 100 ? task.instruction.slice(0, 100) + '...' : task.instruction}
                    </span>
                  </div>
                ))}
              </div>
            ))}
          </Section>

          {/* 계약서 */}
          {proposal.contract && (
            <Section title="인터페이스 계약서">
              <pre style={{
                whiteSpace: 'pre-wrap',
                fontSize: 11,
                lineHeight: 1.6,
                color: 'var(--text-secondary)',
                background: 'var(--bg-primary)',
                padding: 12,
                borderRadius: 6,
                border: '1px solid var(--border)',
                maxHeight: 200,
                overflowY: 'auto',
              }}>
                {proposal.contract}
              </pre>
            </Section>
          )}
        </div>

        {/* 하단 버튼 */}
        <div style={{
          padding: '16px 20px',
          borderTop: '1px solid var(--border)',
          display: 'flex',
          justifyContent: 'flex-end',
          gap: 10,
        }}>
          <button
            onClick={onCancel}
            disabled={executing}
            style={{
              padding: '10px 20px',
              borderRadius: 8,
              border: '1px solid var(--border)',
              background: 'var(--bg-tertiary)',
              color: 'var(--text-secondary)',
              fontSize: 14,
              cursor: 'pointer',
            }}
          >
            취소
          </button>
          <button
            onClick={onExecute}
            disabled={executing}
            style={{
              padding: '10px 24px',
              borderRadius: 8,
              border: 'none',
              background: executing ? 'var(--text-muted)' : 'var(--accent)',
              color: '#1a1b26',
              fontSize: 14,
              fontWeight: 600,
              cursor: executing ? 'default' : 'pointer',
            }}
          >
            {executing ? '실행 중...' : '실행하기'}
          </button>
        </div>
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

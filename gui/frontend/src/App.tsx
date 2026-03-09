import { useState, useCallback, useEffect, useRef } from 'react';
import { EventsOn } from '../wailsjs/runtime/runtime';
import Sidebar from './components/Sidebar';
import LogPanel from './components/LogPanel';
import type { LogEntry } from './components/LogPanel';
import InputBar from './components/InputBar';
import ReportPanel from './components/ReportPanel';
import AgentDetailPanel from './components/AgentDetailPanel';
import type { AgentDetail } from './components/AgentDetailPanel';
import ProposalPanel from './components/ProposalPanel';
import type { Proposal } from './components/ProposalPanel';
import ProjectSetup from './components/ProjectSetup';

import * as WailsApp from '../wailsjs/go/main/App';

interface AgentStatus {
  id: string;
  role: string;
  status: string;
  isConsumer: boolean;
}

interface LogEvent {
  type: 'text' | 'thinking' | 'status';
  message: string;
}

export default function App() {
  const [projectOpen, setProjectOpen] = useState(false);
  const [agents, setAgents] = useState<AgentStatus[]>([]);
  const [selectedAgent, setSelectedAgent] = useState<string | null>(null);
  const [agentDetail, setAgentDetail] = useState<AgentDetail | null>(null);
  const [showDetail, setShowDetail] = useState(false);
  const [logs, setLogs] = useState<LogEntry[]>([{ type: 'text', message: '🎼 Claudestra GUI 시작. 프로젝트를 열거나 생성하세요.' }]);
  const [report, setReport] = useState('');
  const [showReport, setShowReport] = useState(false);
  const [running, setRunning] = useState(false);
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [showProposal, setShowProposal] = useState(false);
  const [executing, setExecuting] = useState(false);

  // 배치 로그 업데이트 (스트리밍 성능 최적화)
  const pendingLogs = useRef<LogEntry[]>([]);
  const flushTimer = useRef<number | null>(null);

  const scheduleFlush = useCallback(() => {
    if (flushTimer.current !== null) return;
    flushTimer.current = requestAnimationFrame(() => {
      flushTimer.current = null;
      if (pendingLogs.current.length === 0) return;
      const batch = pendingLogs.current.splice(0);
      setLogs(prev => [...prev, ...batch]);
    });
  }, []);

  const addLog = useCallback((type: LogEntry['type'], message: string) => {
    pendingLogs.current.push({ type, message });
    scheduleFlush();
  }, [scheduleFlush]);

  const appendLog = useCallback((text: string) => {
    // 마지막 pending 항목에 이어붙이기, 없으면 마지막 로그에
    if (pendingLogs.current.length > 0) {
      const last = pendingLogs.current[pendingLogs.current.length - 1];
      last.message += text;
    } else {
      // pending이 비어있으면 기존 로그의 마지막 항목 업데이트
      setLogs(prev => {
        if (prev.length === 0) return prev;
        const updated = [...prev];
        const last = updated[updated.length - 1];
        updated[updated.length - 1] = { ...last, message: last.message + text };
        return updated;
      });
    }
    scheduleFlush();
  }, [scheduleFlush]);

  // 실시간 이벤트 수신
  useEffect(() => {
    const cancelLog = EventsOn('log', (evt: LogEvent | string) => {
      // 하위 호환: string과 object 모두 처리
      if (typeof evt === 'string') {
        addLog('text', evt);
      } else {
        addLog((evt.type || 'text') as LogEntry['type'], evt.message);
      }
    });
    const cancelAppend = EventsOn('log-append', (text: string) => {
      appendLog(text);
    });
    const cancelTeam = EventsOn('team-updated', (statuses: AgentStatus[]) => {
      if (statuses) setAgents(statuses);
    });
    return () => {
      cancelLog();
      cancelAppend();
      cancelTeam();
      if (flushTimer.current !== null) {
        cancelAnimationFrame(flushTimer.current);
      }
    };
  }, [addLog, appendLog]);

  const refreshStatuses = useCallback(async () => {
    try {
      const statuses = await WailsApp.GetAgentStatuses();
      if (statuses) setAgents(statuses);
    } catch {}
  }, []);

  const handleSelectAgent = useCallback(async (id: string) => {
    setSelectedAgent(id);
    setShowReport(false);
    try {
      const detail = await WailsApp.GetAgentDetail(id);
      if (detail) {
        setAgentDetail(detail);
        setShowDetail(true);
      }
    } catch {
      setAgentDetail(null);
    }
  }, []);

  const handleInit = useCallback(async (dir: string) => {
    try {
      await WailsApp.InitProject(dir);
      setProjectOpen(true);
      addLog('text', `✅ 프로젝트 준비 완료: ${dir}`);
      addLog('text', '요구사항을 입력하면 팀장이 분석 후 실행 계획을 제안합니다.');
    } catch (e: any) {
      addLog('text', `❌ 초기화 실패: ${e}`);
    }
  }, [addLog]);

  const handleOpen = useCallback(async (dir: string) => {
    try {
      await WailsApp.OpenProject(dir);
      setProjectOpen(true);
      addLog('text', `✅ 프로젝트 열기 완료: ${dir}`);
      await refreshStatuses();
    } catch (e: any) {
      addLog('text', `❌ 프로젝트 열기 실패: ${e}`);
    }
  }, [addLog, refreshStatuses]);

  // 1단계: 계획 수립 요청
  const handleSubmit = useCallback(async (input: string) => {
    setRunning(true);
    addLog('text', `\n📝 요청: ${input}`);
    try {
      const result = await WailsApp.PlanRequest(input);
      if (result) {
        setProposal(result);
        setShowProposal(true);
        setShowReport(false);
        setShowDetail(false);
        addLog('text', '[팀장] 실행 계획을 수립했습니다. 우측 상단 [실행 계획] 버튼으로 검토하세요.');
      }
    } catch (e: any) {
      const errStr = String(e);
      if (errStr.startsWith('DIRECT_REPLY:')) {
        // 스트리밍으로 이미 출력됨 — 중복 로그 방지
      } else {
        addLog('text', `❌ 오류: ${e}`);
      }
    }
    setRunning(false);
  }, [addLog]);

  // 2단계: 승인 후 실행
  const handleExecute = useCallback(async () => {
    setExecuting(true);
    setShowProposal(false);
    addLog('text', '\n[팀장] 실행 승인됨. 작업을 시작합니다...');
    try {
      const result = await WailsApp.ExecutePlan();
      setReport(result);
      setShowReport(true);
      addLog('text', '\n✅ 작업 완료 — 보고서를 확인하세요.');
      await refreshStatuses();
    } catch (e: any) {
      addLog('text', `❌ 실행 오류: ${e}`);
    }
    setExecuting(false);
    setProposal(null);
  }, [addLog, refreshStatuses]);

  const handleCancelProposal = useCallback(() => {
    setShowProposal(false);
    setProposal(null);
    addLog('text', '[사용자] 실행 취소.');
  }, [addLog]);

  const handleSelectDir = useCallback(async () => {
    try {
      return await WailsApp.SelectDirectory();
    } catch {
      return '';
    }
  }, []);

  if (!projectOpen) {
    return (
      <ProjectSetup
        onInit={handleInit}
        onOpen={handleOpen}
        onSelectDir={handleSelectDir}
      />
    );
  }

  return (
    <div style={{ display: 'flex', height: '100%' }}>
      <Sidebar
        agents={agents}
        onSelectAgent={handleSelectAgent}
        selectedAgent={selectedAgent}
      />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', position: 'relative' }}>
        {/* 헤더 */}
        <div style={{
          padding: '10px 16px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--bg-secondary)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          fontSize: 13,
        }}>
          <span style={{ color: 'var(--text-muted)' }}>
            팀원 {agents.length}명 | {agents.filter(a => a.status === 'RUNNING').length}명 실행 중
          </span>
          <div style={{ display: 'flex', gap: 8 }}>
            {proposal && (
              <button
                onClick={() => { setShowProposal(!showProposal); setShowReport(false); setShowDetail(false); }}
                style={{
                  padding: '4px 12px',
                  borderRadius: 4,
                  border: '1px solid var(--border)',
                  background: showProposal ? 'var(--accent)' : 'var(--bg-tertiary)',
                  color: showProposal ? '#1a1b26' : 'var(--accent)',
                  cursor: 'pointer',
                  fontSize: 12,
                  fontWeight: showProposal ? 600 : 400,
                }}
              >
                실행 계획
              </button>
            )}
            {report && (
              <button
                onClick={() => { setShowReport(!showReport); setShowProposal(false); setShowDetail(false); }}
                style={{
                  padding: '4px 12px',
                  borderRadius: 4,
                  border: '1px solid var(--border)',
                  background: showReport ? 'var(--accent)' : 'var(--bg-tertiary)',
                  color: showReport ? '#1a1b26' : 'var(--accent)',
                  cursor: 'pointer',
                  fontSize: 12,
                  fontWeight: showReport ? 600 : 400,
                }}
              >
                📋 보고서
              </button>
            )}
          </div>
        </div>

        {/* 로그 */}
        <LogPanel logs={logs} />

        {/* 입력 */}
        <InputBar onSubmit={handleSubmit} disabled={running || executing} />

        {/* 실행 계획 제안 패널 */}
        <ProposalPanel
          proposal={proposal}
          visible={showProposal}
          onExecute={handleExecute}
          onCancel={handleCancelProposal}
          executing={executing}
        />

        {/* 보고서 패널 */}
        <ReportPanel
          report={report}
          visible={showReport}
          onClose={() => setShowReport(false)}
        />

        {/* 에이전트 상세 패널 */}
        <AgentDetailPanel
          detail={agentDetail}
          visible={showDetail && !showReport && !showProposal}
          onClose={() => { setShowDetail(false); setSelectedAgent(null); }}
        />
      </div>
    </div>
  );
}

"""
Orchestra - Lead Agent (팀장)
사용자 입력을 분석해서 각 팀원에게 태스크를 배분합니다.
"""

import subprocess
import json
import re
from pathlib import Path
from typing import Optional

from agent import Agent, AgentConfig, AgentStatus


class LeadAgent:
    """
    팀장 AI.
    1. 사용자 입력을 받아 태스크를 분해합니다.
    2. 각 팀원에게 적절한 지시를 배분합니다.
    3. 팀원 완료 후 결과를 수집하고 보고합니다.
    """

    IDEA = """당신은 소프트웨어 개발 팀의 팀장 AI입니다.
당신의 역할은:
1. 사용자의 요구사항을 분석합니다.
2. 각 팀원의 역할에 맞게 태스크를 분해합니다.
3. 팀원들의 결과물을 취합하여 최종 보고를 작성합니다.

반드시 JSON 형식으로만 응답하세요. 다른 텍스트는 포함하지 마세요."""

    def __init__(self, work_dir: Path):
        self.work_dir  = work_dir
        self.agents: dict[str, Agent] = {}
        self.work_dir.mkdir(parents=True, exist_ok=True)

    # ── 팀원 관리 ──────────────────────────────────────────────

    def add_agent(self, agent: Agent):
        self.agents[agent.config.agent_id] = agent
        print(f"[팀장] 팀원 추가: {agent.config.role} (id={agent.config.agent_id})")

    def list_agents(self) -> list[dict]:
        return [
            {
                "id":     a.config.agent_id,
                "role":   a.config.role,
                "status": a.status.value,
            }
            for a in self.agents.values()
        ]

    # ── 핵심: 사용자 입력 처리 ─────────────────────────────────

    def process(self, user_input: str) -> str:
        """
        사용자 입력 → 태스크 분배 → 팀원 실행 → 결과 수집 → 최종 보고
        """
        print(f"\n{'='*60}")
        print(f"[팀장] 사용자 입력 수신: {user_input}")
        print(f"{'='*60}")

        # 1단계: 태스크 분해
        print("\n[팀장] 📋 태스크 분해 중...")
        tasks = self._decompose(user_input)
        if not tasks:
            return "태스크 분해에 실패했습니다."

        print(f"[팀장] 총 {len(tasks)}개 태스크 생성:")
        for t in tasks:
            agent_id = t.get("agent_id", "?")
            desc     = t.get("instruction", "")[:60]
            mode     = "⚡병렬" if t.get("parallel") else "🔗순차"
            print(f"  {mode} [{agent_id}] {desc}...")

        # 2단계: 병렬 / 순차 실행
        results = self._execute_tasks(tasks)

        # 3단계: 결과 수집 & 최종 보고
        print("\n[팀장] 📝 최종 보고 작성 중...")
        report = self._summarize(user_input, results)
        return report

    # ── 내부: 태스크 분해 ──────────────────────────────────────

    def _decompose(self, user_input: str) -> list[dict]:
        """
        Claude Code를 사용해 user_input을 팀원별 태스크로 분해합니다.
        반환 형식:
        [
          {
            "agent_id": "backend",
            "instruction": "...",
            "parallel": true   // false면 앞 태스크 완료 후 실행
          },
          ...
        ]
        """
        agent_list = json.dumps(self.list_agents(), ensure_ascii=False, indent=2)

        prompt = f"""{self.IDEA}

[현재 팀원 목록]
{agent_list}

[사용자 요구사항]
{user_input}

위 요구사항을 팀원들에게 배분할 태스크 목록으로 분해하세요.
각 팀원의 역할에 맞는 구체적인 지시를 작성하세요.
parallel이 true면 다른 태스크와 동시에 실행, false면 직전 태스크 완료 후 실행합니다.

반드시 아래 JSON 배열 형식으로만 응답하세요:
[
  {{
    "agent_id": "팀원ID",
    "instruction": "구체적인 지시 내용",
    "parallel": true
  }}
]"""

        try:
            result = subprocess.run(
                ["claude", "--print", "--dangerously-skip-permissions", prompt],
                cwd=str(self.work_dir),
                capture_output=True,
                text=True,
                timeout=120,
            )

            if result.returncode != 0:
                print(f"[팀장] ❌ 태스크 분해 오류: {result.stderr[:200]}")
                return self._fallback_decompose(user_input)

            raw = result.stdout.strip()
            # JSON 블록 추출 (```json ... ``` 감싸진 경우도 처리)
            raw = re.sub(r"```json\s*|\s*```", "", raw).strip()
            tasks = json.loads(raw)

            # agent_id 유효성 검사
            valid_ids = set(self.agents.keys())
            tasks = [t for t in tasks if t.get("agent_id") in valid_ids]
            return tasks

        except (json.JSONDecodeError, FileNotFoundError) as e:
            print(f"[팀장] ⚠️  파싱 실패 ({e}), 폴백 모드 사용")
            return self._fallback_decompose(user_input)

    def _fallback_decompose(self, user_input: str) -> list[dict]:
        """Claude Code 없이 동작하는 폴백: 모든 팀원에게 동일 지시."""
        print("[팀장] ⚠️  폴백: 모든 팀원에게 동일 지시 전달")
        return [
            {
                "agent_id":    agent_id,
                "instruction": f"다음 요구사항에서 당신의 역할({agent.config.role})에 해당하는 부분을 수행하세요:\n\n{user_input}",
                "parallel":    True,
            }
            for agent_id, agent in self.agents.items()
        ]

    # ── 내부: 태스크 실행 ──────────────────────────────────────

    def _execute_tasks(self, tasks: list[dict]) -> dict[str, str]:
        """
        parallel=True인 태스크는 동시 실행,
        parallel=False인 태스크는 직전 배치 완료 후 실행.
        """
        results: dict[str, str] = {}
        pending_threads: list[tuple[str, object]] = []

        for task in tasks:
            agent_id    = task["agent_id"]
            instruction = task["instruction"]
            is_parallel = task.get("parallel", True)
            agent       = self.agents[agent_id]

            if not is_parallel and pending_threads:
                # 앞선 배치 전부 완료 대기
                print(f"\n[팀장] ⏳ 순차 태스크 대기 중...")
                for aid, t in pending_threads:
                    t.join(timeout=300)
                    results[aid] = self.agents[aid].output
                pending_threads.clear()

            agent.reset()
            t = agent.run_async(instruction)
            pending_threads.append((agent_id, t))

        # 나머지 전부 대기
        for aid, t in pending_threads:
            t.join(timeout=300)
            results[aid] = self.agents[aid].output

        return results

    # ── 내부: 결과 요약 ────────────────────────────────────────

    def _summarize(self, user_input: str, results: dict[str, str]) -> str:
        """팀원 결과물을 취합해 최종 보고서를 작성합니다."""
        results_text = "\n\n".join(
            f"[{self.agents[aid].config.role} 결과]\n{output}"
            for aid, output in results.items()
            if aid in self.agents
        )

        prompt = f"""당신은 소프트웨어 개발 팀의 팀장입니다.
팀원들의 작업 결과를 취합하여 사용자에게 전달할 최종 보고서를 작성하세요.

[원래 사용자 요청]
{user_input}

[팀원별 작업 결과]
{results_text}

위 내용을 바탕으로:
1. 완료된 작업 요약
2. 각 팀원이 수행한 내용
3. 전체 결과물에 대한 평가
4. 다음 단계 제안

형식으로 한국어 보고서를 작성하세요."""

        try:
            result = subprocess.run(
                ["claude", "--print", "--dangerously-skip-permissions", prompt],
                cwd=str(self.work_dir),
                capture_output=True,
                text=True,
                timeout=120,
            )
            if result.returncode == 0:
                return result.stdout.strip()
        except FileNotFoundError:
            pass

        # 폴백: 단순 결합
        return f"[팀원 결과 요약]\n\n{results_text}"

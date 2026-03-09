"""
Orchestra - Agent Base Classes
각 에이전트는 독립된 subprocess로 Claude Code를 실행합니다.
"""

import subprocess
import threading
import time
import os
import json
from pathlib import Path
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class AgentStatus(Enum):
    IDLE    = "idle"
    RUNNING = "running"
    DONE    = "done"
    ERROR   = "error"


@dataclass
class AgentConfig:
    agent_id:   str
    role:       str
    idea:       str          # 시스템 프롬프트 (이데아)
    work_dir:   Path
    read_refs:  list[Path] = field(default_factory=list)  # 읽기 전용 참조 경로


class Agent:
    """
    단일 Claude Code 인스턴스를 래핑하는 에이전트.
    claude --print 를 subprocess로 실행하고 결과를 캡처합니다.
    """

    def __init__(self, config: AgentConfig):
        self.config  = config
        self.status  = AgentStatus.IDLE
        self.output  = ""       # 마지막 실행 결과
        self._lock   = threading.Lock()

        # 작업 디렉토리 생성
        self.config.work_dir.mkdir(parents=True, exist_ok=True)
        self._write_status("IDLE")

    # ── 공개 API ──────────────────────────────────────────────

    def run(self, instruction: str) -> str:
        """
        에이전트에게 지시를 내리고 결과를 반환합니다.
        blocking 방식 (완료까지 대기).
        """
        with self._lock:
            self.status = AgentStatus.RUNNING
            self._write_status("RUNNING")

        prompt = self._build_prompt(instruction)

        print(f"\n[{self.config.role}] 🚀 시작: {instruction[:60]}...")

        try:
            result = subprocess.run(
                ["claude", "--print", "--dangerously-skip-permissions", prompt],
                cwd=str(self.config.work_dir),
                capture_output=True,
                text=True,
                timeout=300,  # 5분 타임아웃
            )

            if result.returncode == 0:
                self.output = result.stdout.strip()
                self.status = AgentStatus.DONE
                self._write_status("DONE")
                print(f"[{self.config.role}] ✅ 완료")
            else:
                self.output = result.stderr.strip()
                self.status = AgentStatus.ERROR
                self._write_status("ERROR")
                print(f"[{self.config.role}] ❌ 오류: {self.output[:100]}")

        except subprocess.TimeoutExpired:
            self.status = AgentStatus.ERROR
            self._write_status("ERROR")
            self.output = "TIMEOUT: 5분 초과"
            print(f"[{self.config.role}] ⏰ 타임아웃")

        except FileNotFoundError:
            self.status = AgentStatus.ERROR
            self._write_status("ERROR")
            self.output = "ERROR: 'claude' 명령어를 찾을 수 없습니다. Claude Code가 설치되어 있나요?"
            print(f"[{self.config.role}] ❌ {self.output}")

        return self.output

    def run_async(self, instruction: str) -> threading.Thread:
        """비동기 실행 — 스레드를 반환합니다."""
        t = threading.Thread(target=self.run, args=(instruction,), daemon=True)
        t.start()
        return t

    def wait_until_done(self, timeout: float = 300) -> bool:
        """완료될 때까지 폴링으로 대기. True = 성공, False = 타임아웃/에러."""
        deadline = time.time() + timeout
        while time.time() < deadline:
            if self.status in (AgentStatus.DONE, AgentStatus.ERROR):
                return self.status == AgentStatus.DONE
            time.sleep(1)
        return False

    def reset(self):
        self.status = AgentStatus.IDLE
        self.output = ""
        self._write_status("IDLE")

    # ── 내부 헬퍼 ─────────────────────────────────────────────

    def _build_prompt(self, instruction: str) -> str:
        """이데아 + 크로스 참조 경로 + 실제 지시를 합칩니다."""
        parts = [self.config.idea]

        if self.config.read_refs:
            refs_text = "\n".join(f"  - {p}" for p in self.config.read_refs)
            parts.append(f"[읽기 전용 참조 디렉토리 — 수정 금지]\n{refs_text}")

        parts.append(f"[지시]\n{instruction}")
        return "\n\n".join(parts)

    def _write_status(self, status: str):
        status_file = self.config.work_dir / ".agent-status"
        status_file.write_text(status)

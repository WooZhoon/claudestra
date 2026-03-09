"""
Orchestra MVP - 메인 진입점
사용법:
  python main.py init    # 새 프로젝트 초기화
  python main.py run     # 오케스트레이션 실행
  python main.py idea    # 이데아 편집
  python main.py status  # 팀원 상태 확인
"""

import sys
import argparse
import unicodedata
from pathlib import Path

import yaml

from agent      import Agent, AgentConfig
from lead_agent import LeadAgent
from workspace  import Workspace, CONSUMER_ROLES
from file_lock  import FileLockRegistry


# ── 역할별 허용 도구 ────────────────────────────────────────────────

# Producer (코드 작성자): 파일 읽기/쓰기/검색 + Bash
PRODUCER_TOOLS = ["Read", "Write", "Edit", "Glob", "Grep", "Bash"]

# Consumer (리뷰어 등): 읽기 전용 — 코드를 수정하지 못하게 제한
CONSUMER_TOOLS = ["Read", "Glob", "Grep"]

ROLE_TOOLS: dict[str, list[str]] = {
    "backend":    PRODUCER_TOOLS,
    "frontend":   PRODUCER_TOOLS,
    "db":         PRODUCER_TOOLS,
    "reviewer":   CONSUMER_TOOLS,
    "doc_writer": CONSUMER_TOOLS + ["Write"],  # 문서 작성만 허용
}


# ── 팩토리: 에이전트 생성 ──────────────────────────────────────────

def build_team(workspace: Workspace, roles: list[str]) -> tuple[LeadAgent, dict]:
    """
    워크스페이스 설정을 기반으로 팀장 + 팀원을 생성합니다.
    """
    lock_registry = FileLockRegistry(workspace.locks_dir)
    lead = LeadAgent(work_dir=workspace.root)
    agents = {}

    for role in roles:
        idea      = workspace.load_idea(role)
        agent_dir = workspace.root / role

        # Consumer는 Producer 디렉토리를 읽기 전용 참조
        read_refs = []
        if workspace.is_consumer(role):
            read_refs = workspace.get_producer_dirs(exclude_role=role)

        # 역할별 허용 도구 (미등록 역할은 제한 없음)
        allowed_tools = ROLE_TOOLS.get(role, [])

        config = AgentConfig(
            agent_id      = role,
            role          = role,
            idea          = idea,
            work_dir      = agent_dir,
            read_refs     = read_refs,
            allowed_tools = allowed_tools,
        )
        agent = Agent(config, lock_registry=lock_registry)
        agents[role] = agent
        lead.add_agent(agent)

    return lead, agents


# ── 커맨드 핸들러 ─────────────────────────────────────────────────

def cmd_init(args):
    """새 Orchestra 프로젝트를 초기화합니다."""
    project_dir = Path(args.project_dir)
    workspace   = Workspace(project_dir)

    print("\n🎼 Orchestra 프로젝트 초기화")
    print("─" * 40)

    # 역할 선택
    print("추가할 팀원 역할을 선택하세요 (쉼표로 구분):")
    print("사용 가능: backend, frontend, db, reviewer, doc_writer")
    print("예시: backend,frontend,db,reviewer")
    print()

    if hasattr(args, 'roles') and args.roles:
        roles = [r.strip() for r in args.roles.split(",")]
    else:
        raw = input("역할 입력 > ").strip()
        roles = [r.strip() for r in raw.split(",") if r.strip()]

    if not roles:
        print("❌ 역할을 입력해주세요.")
        return

    workspace.init(roles)
    print(f"\n✅ 완료! 프로젝트 경로: {project_dir.resolve()}")
    print(f"다음 명령어로 실행하세요:")
    print(f"  python main.py run --project {project_dir}")


def cmd_run(args):
    """팀장 AI에게 요청을 전달하고 오케스트레이션을 실행합니다."""
    project_dir = Path(args.project_dir)
    workspace   = Workspace(project_dir)

    config_file = workspace.orchestra / "config.yaml"
    if not config_file.exists():
        print(f"❌ 초기화된 프로젝트가 없습니다. 먼저 'init'을 실행하세요.")
        return

    config = yaml.safe_load(config_file.read_text())
    roles  = config.get("agents", [])

    if not roles:
        print("❌ 등록된 팀원이 없습니다.")
        return

    # 팀 구성
    lead, agents = build_team(workspace, roles)

    print("\n🎼 Orchestra 실행")
    print("─" * 40)
    print(f"팀원: {', '.join(roles)}")
    print("─" * 40)

    # 사용자 입력 루프
    while True:
        print()
        try:
            user_input = input("📝 요청 입력 (종료: q) > ")
        except (EOFError, KeyboardInterrupt):
            print("\n👋 종료합니다.")
            break

        # 보이지 않는 제어 문자 제거 (한글 등 유니코드는 유지)
        user_input = "".join(ch for ch in user_input if ch == "\n" or not unicodedata.category(ch).startswith("C")).strip()

        if not user_input or user_input.lower() in ("q", "quit", "exit"):
            if not user_input:
                continue
            print("👋 종료합니다.")
            break

        report = lead.process(user_input)

        print("\n" + "=" * 60)
        print("📋 최종 보고서")
        print("=" * 60)
        print(report)
        print("=" * 60)


def cmd_idea(args):
    """특정 에이전트의 이데아를 편집합니다."""
    project_dir = Path(args.project_dir)
    workspace   = Workspace(project_dir)

    config_file = workspace.orchestra / "config.yaml"
    if not config_file.exists():
        print("❌ 초기화된 프로젝트가 없습니다.")
        return

    config = yaml.safe_load(config_file.read_text())
    roles  = config.get("agents", [])

    print("\n🧠 이데아 편집")
    print("─" * 40)
    print(f"등록된 역할: {', '.join(roles)}")
    role = input("편집할 역할 > ").strip()

    if role not in roles:
        print(f"❌ '{role}' 역할을 찾을 수 없습니다.")
        return

    current = workspace.load_idea(role)
    print(f"\n[현재 이데아 - {role}]")
    print("─" * 40)
    print(current)
    print("─" * 40)
    print("\n새 이데아를 입력하세요 (빈 줄 + END 로 종료):")

    lines = []
    while True:
        line = input()
        if line.strip() == "END":
            break
        lines.append(line)

    new_idea = "\n".join(lines).strip()
    if new_idea:
        workspace.save_idea(role, new_idea)
        print(f"✅ '{role}' 이데아가 업데이트되었습니다.")
    else:
        print("⚠️  변경사항 없음.")


def cmd_status(args):
    """현재 팀원들의 상태를 확인합니다."""
    project_dir = Path(args.project_dir)
    workspace   = Workspace(project_dir)

    config_file = workspace.orchestra / "config.yaml"
    if not config_file.exists():
        print("❌ 초기화된 프로젝트가 없습니다.")
        return

    config = yaml.safe_load(config_file.read_text())
    roles  = config.get("agents", [])

    print("\n📊 팀원 상태")
    print("─" * 50)

    for role in roles:
        agent_dir   = project_dir / role
        status_file = agent_dir / ".agent-status"
        status      = status_file.read_text().strip() if status_file.exists() else "UNKNOWN"
        idea_size   = len(workspace.load_idea(role))
        is_consumer = workspace.is_consumer(role)
        role_type   = "Consumer" if is_consumer else "Producer"

        icon = {"IDLE": "⚪", "RUNNING": "🔵", "DONE": "✅", "ERROR": "❌"}.get(status, "❓")
        print(f"  {icon} {role:<15} [{role_type:<10}]  상태: {status:<10}  이데아: {idea_size}자")

    # 활성 잠금 표시
    lock_registry = FileLockRegistry(workspace.locks_dir)
    locks = lock_registry.list_locks()
    if locks:
        print()
        print("🔒 활성 잠금")
        print("─" * 50)
        for path, agent_id in locks.items():
            print(f"  {agent_id:<15} → {path}")

    print("─" * 50)


# ── 메인 ──────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="🎼 Orchestra - Claude Code Multi-Agent Orchestration MVP"
    )

    subparsers = parser.add_subparsers(dest="command")

    # 공통 인자
    parent = argparse.ArgumentParser(add_help=False)
    parent.add_argument(
        "--project", "-p",
        dest="project_dir",
        default="./my-orchestra-project",
        help="프로젝트 디렉토리 경로 (기본값: ./my-orchestra-project)"
    )

    # init
    p_init = subparsers.add_parser("init", parents=[parent], help="새 프로젝트 초기화")
    p_init.add_argument("--roles", help="역할 목록 (쉼표 구분, 예: backend,frontend,db)")

    # run
    subparsers.add_parser("run", parents=[parent], help="오케스트레이션 실행")

    # idea
    subparsers.add_parser("idea", parents=[parent], help="에이전트 이데아 편집")

    # status
    subparsers.add_parser("status", parents=[parent], help="팀원 상태 확인")

    args = parser.parse_args()

    if args.command == "init":
        cmd_init(args)
    elif args.command == "run":
        cmd_run(args)
    elif args.command == "idea":
        cmd_idea(args)
    elif args.command == "status":
        cmd_status(args)
    else:
        print("🎼 Orchestra MVP")
        print("사용법: python main.py [init|run|idea|status] --project <경로>")
        print()
        parser.print_help()


if __name__ == "__main__":
    main()

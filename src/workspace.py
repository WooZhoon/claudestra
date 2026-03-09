"""
Orchestra - Workspace Manager
프로젝트 디렉토리 구조 생성 및 이데아 관리
"""

import yaml
from pathlib import Path
from dataclasses import dataclass


# ── 기본 이데아 템플릿 ────────────────────────────────────────────

DEFAULT_IDEAS: dict[str, str] = {
    "backend": """당신은 백엔드 개발 전문가입니다.
담당 디렉토리: ./backend/

전문 영역:
- RESTful API 설계 및 구현
- 인증/인가 시스템 (JWT, OAuth2)
- 비즈니스 로직 구현
- 성능 최적화 및 캐싱 전략

작업 규칙:
- 자신의 디렉토리(./backend/) 외부 파일은 절대 수정하지 않습니다.
- 작업 완료 시 ./backend/.agent-status 에 DONE을 기록합니다.
- 다른 팀원과의 인터페이스가 필요하면 주석으로 명확히 명세합니다.""",

    "frontend": """당신은 프론트엔드 개발 전문가입니다.
담당 디렉토리: ./frontend/

전문 영역:
- React / Vue 컴포넌트 설계
- 상태 관리 (Zustand, Redux)
- UI/UX 구현 및 반응형 디자인
- API 연동

작업 규칙:
- 자신의 디렉토리(./frontend/) 외부 파일은 절대 수정하지 않습니다.
- 작업 완료 시 ./frontend/.agent-status 에 DONE을 기록합니다.
- 백엔드 API 명세가 필요하면 주석에 가정사항을 명확히 기록합니다.""",

    "db": """당신은 데이터베이스 전문가입니다.
담당 디렉토리: ./db/

전문 영역:
- 데이터베이스 스키마 설계
- SQL 쿼리 최적화
- 마이그레이션 스크립트 작성
- 인덱스 전략

작업 규칙:
- 자신의 디렉토리(./db/) 외부 파일은 절대 수정하지 않습니다.
- 작업 완료 시 ./db/.agent-status 에 DONE을 기록합니다.
- 모든 스키마 변경에는 롤백 스크립트를 함께 작성합니다.""",

    "reviewer": """당신은 코드 리뷰 전문가입니다.
담당 디렉토리: ./reviewer/

전문 영역:
- 코드 품질 분석
- 보안 취약점 탐지
- 성능 이슈 식별
- 코딩 컨벤션 검토

작업 규칙:
- 읽기 전용으로 다른 팀원의 코드를 참조합니다.
- 자신의 디렉토리(./reviewer/)에만 리뷰 결과를 작성합니다.
- 작업 완료 시 ./reviewer/.agent-status 에 DONE을 기록합니다.
- 비판보다 건설적인 개선 제안을 작성합니다.""",

    "doc_writer": """당신은 기술 문서 작성 전문가입니다.
담당 디렉토리: ./docs/

전문 영역:
- API 문서 작성 (OpenAPI/Swagger)
- README 및 가이드 문서
- 아키텍처 문서
- 코드 주석 정리

작업 규칙:
- 읽기 전용으로 다른 팀원의 코드를 참조합니다.
- 자신의 디렉토리(./docs/)에만 문서를 작성합니다.
- 작업 완료 시 ./docs/.agent-status 에 DONE을 기록합니다.
- 개발자와 비개발자 모두 이해할 수 있도록 문서를 작성합니다.""",
}

CONSUMER_ROLES = {"reviewer", "doc_writer"}  # 크로스 참조 에이전트


# ── Workspace 클래스 ───────────────────────────────────────────────

class Workspace:
    """
    프로젝트 워크스페이스를 초기화하고 관리합니다.
    .orchestra/ 디렉토리에 설정과 이데아를 저장합니다.
    """

    def __init__(self, root: Path):
        self.root        = root
        self.orchestra   = root / ".orchestra"
        self.ideas_dir   = self.orchestra / "ideas"

        self.orchestra.mkdir(parents=True, exist_ok=True)
        self.ideas_dir.mkdir(parents=True, exist_ok=True)

    def init(self, agent_roles: list[str]):
        """워크스페이스와 각 에이전트 디렉토리를 초기화합니다."""
        config = {"agents": agent_roles, "version": "1.0"}
        (self.orchestra / "config.yaml").write_text(
            yaml.dump(config, allow_unicode=True)
        )

        for role in agent_roles:
            # 서브 디렉토리 생성
            agent_dir = self.root / role
            agent_dir.mkdir(exist_ok=True)

            # 이데아 파일 생성 (없으면 기본값)
            idea_file = self.ideas_dir / f"{role}.yaml"
            if not idea_file.exists():
                idea_text = DEFAULT_IDEAS.get(role, f"당신은 {role} 전문가입니다.\n담당 디렉토리: ./{role}/")
                idea_file.write_text(
                    yaml.dump({"role": role, "idea": idea_text}, allow_unicode=True)
                )

        print(f"[워크스페이스] ✅ 초기화 완료: {self.root}")
        print(f"[워크스페이스] 에이전트: {', '.join(agent_roles)}")

    def load_idea(self, role: str) -> str:
        """이데아 파일에서 시스템 프롬프트를 로드합니다."""
        idea_file = self.ideas_dir / f"{role}.yaml"
        if idea_file.exists():
            data = yaml.safe_load(idea_file.read_text())
            return data.get("idea", "")
        return DEFAULT_IDEAS.get(role, f"당신은 {role} 전문가입니다.")

    def save_idea(self, role: str, idea: str):
        """이데아를 파일에 저장합니다 (사용자 편집 지원)."""
        idea_file = self.ideas_dir / f"{role}.yaml"
        idea_file.write_text(
            yaml.dump({"role": role, "idea": idea}, allow_unicode=True)
        )
        print(f"[워크스페이스] 이데아 저장: {role}")

    def get_producer_dirs(self, exclude_role: str = "") -> list[Path]:
        """Consumer 에이전트가 참조할 Producer 디렉토리 목록을 반환합니다."""
        config_file = self.orchestra / "config.yaml"
        if not config_file.exists():
            return []

        config = yaml.safe_load(config_file.read_text())
        roles  = config.get("agents", [])

        return [
            self.root / role
            for role in roles
            if role not in CONSUMER_ROLES and role != exclude_role
        ]

    def is_consumer(self, role: str) -> bool:
        return role in CONSUMER_ROLES

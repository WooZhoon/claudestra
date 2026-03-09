FROM python:3.12-slim

# Node.js 설치 (Claude Code CLI 용)
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl ca-certificates && \
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Claude Code CLI 설치
RUN npm install -g @anthropic-ai/claude-code

# Python 의존성
RUN pip install --no-cache-dir pyyaml

# 일반 유저 생성 (root에서 claude 실행 불가)
RUN useradd -m -s /bin/bash orchestra
RUN mkdir -p /app /workspace && chown -R orchestra:orchestra /app /workspace

# 소스 복사
WORKDIR /app
COPY --chown=orchestra:orchestra src/ /app/

USER orchestra
ENV PYTHONDONTWRITEBYTECODE=1

CMD ["bash"]

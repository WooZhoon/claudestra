#!/bin/bash
set -e

cd "$(dirname "$0")"

IMAGE_NAME="orchestra-mvp"

# 빌드
echo "🔨 Docker 이미지 빌드 중..."
docker build -t "$IMAGE_NAME" .

# 컨테이너 실행
echo ""
echo "🎼 Orchestra 컨테이너 시작"
echo "─────────────────────────────────────"
echo "사용법:"
echo "  python main.py init -p /workspace/myproject --roles backend,frontend,db,reviewer"
echo "  python main.py run  -p /workspace/myproject"
echo "  python main.py status -p /workspace/myproject"
echo "─────────────────────────────────────"
echo ""

docker run -it --rm \
    -v "$HOME/.claude/.credentials.json:/home/orchestra/.claude/.credentials.json:ro" \
    -v "$HOME/.claude.json:/home/orchestra/.claude.json:ro" \
    -v "$(pwd)/src:/app" \
    "$IMAGE_NAME" \
    bash

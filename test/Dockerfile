# 結合テスト用の mcadmind の像。
#
# **本番はこの像を使わない。** 本番は systemd 配下のホストプロセスで、
# docker.sock をコンテナへ渡さない（渡すのは実質 root 権限を渡すのと
# 変わらないため）。これはテスト環境を 1 コマンドで立てるためだけのもの。
#
# ビルドの文脈はリポジトリのルート。frontend と backend の両方が要る。

# --- フロントエンドを Go の埋め込み先へ出す ---
FROM node:22-alpine AS web
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN cd frontend && npm ci
COPY frontend ./frontend
COPY proto ./proto
COPY backend/internal/presentation/webui ./backend/internal/presentation/webui
# vite.config.ts の build.outDir が backend 側を指しているので、そのまま出る。
RUN cd frontend && npx vite build

# --- バイナリ ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend ./backend
# 埋め込みの中身を web 段から持ってくる。
COPY --from=web /src/backend/internal/presentation/webui/dist ./backend/internal/presentation/webui/dist
RUN cd backend && CGO_ENABLED=0 go build -ldflags '-s -w' -o /out/mcadmind ./cmd/mcadmind

# --- 実行 ---
FROM alpine:3.21
# mcadmind は docker compose を子プロセスとして起動する。像に入れておく。
RUN apk add --no-cache docker-cli docker-cli-compose ca-certificates
COPY --from=build /out/mcadmind /usr/local/bin/mcadmind
# -project-dir は compose 側から渡す。ホストと同じ絶対パスを指す必要がある。
ENTRYPOINT ["/usr/local/bin/mcadmind"]

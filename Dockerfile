# syntax=docker/dockerfile:1
ARG APP_VERSION=unknown

# ---------- Stage 1: build frontend ----------
FROM node:20-alpine AS frontend-build
# 腾讯云 npm 镜像：境内服务器直连 npmjs.org 很慢，npm ci 是构建耗时大头
RUN npm config set registry https://registry.npmmirror.com
WORKDIR /build
COPY frontend/package.json frontend/package-lock.json ./
# npm 缓存挂载：依赖不变时 npm ci 秒级完成
RUN --mount=type=cache,target=/root/.npm npm ci
COPY frontend/ ./
RUN npm run build

# ---------- Stage 2: build backend ----------
FROM golang:1.25-alpine AS backend-build
ARG APP_VERSION
# goproxy.cn：境内拉 Go 模块，proxy.golang.org 直连基本不可用
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
# 模块缓存 + 编译缓存挂载：改代码重编译从 40s 降到几秒，且不进镜像层
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,sharing=locked,target=/root/.cache/go-build \
    go mod download
COPY backend/ ./
# 前端产物放进 backend/static，与 build_linux.bat 的产物布局一致
COPY --from=frontend-build /build/dist ./static
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,sharing=locked,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/ninimenu ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/dbmigrate ./cmd/dbmigrate

# ---------- Stage 3: bundled CPU speech runtime (no separate service) ----------
FROM python:3.12-slim-bookworm AS asr-runtime
RUN sed -i 's|http://deb.debian.org/debian|https://mirrors.cloud.tencent.com/debian|g' /etc/apt/sources.list.d/debian.sources \
    && apt-get -o Acquire::Retries=0 -o Acquire::https::Timeout=30 update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata ffmpeg libgomp1 libseccomp2 wget \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 1000 --create-home ninimenu \
    && mkdir -p /app/data /app/uploads /app/video-work \
    && chown -R ninimenu:ninimenu /app
COPY backend/video_asr/requirements.txt /opt/video-asr-requirements.txt
RUN python3 -m venv /opt/video-asr \
    && /opt/video-asr/bin/pip install --index-url https://pypi.tuna.tsinghua.edu.cn/simple --retries 1 --timeout 30 --no-cache-dir --only-binary=:all: --require-hashes -r /opt/video-asr-requirements.txt
COPY backend/video_asr/transcribe.py backend/video_asr/sandbox.py /app/video-asr/
COPY backend/video_asr/licenses/ /app/video-asr/licenses/
COPY backend/internal/video/model.lock.json /app/video-asr/model.lock.json
ENV OPENBLAS_NUM_THREADS=1 OMP_NUM_THREADS=2 MKL_NUM_THREADS=1 MALLOC_ARENA_MAX=2
WORKDIR /app

# ---------- Stage 4: existing web/API application ----------
FROM asr-runtime
ARG APP_VERSION
COPY --from=backend-build /out/ninimenu ./ninimenu
COPY --from=backend-build /out/dbmigrate ./dbmigrate
# 前端产物（SPA）
COPY --from=backend-build /src/static ./static
# 种子菜品图片；首次挂载空具名卷时由 Docker 自动拷贝进卷
COPY --chown=ninimenu:ninimenu backend/uploads ./uploads
ENV PORT=8080 GIN_MODE=release TZ=Asia/Shanghai APP_VERSION=$APP_VERSION
EXPOSE 8080
USER ninimenu
VOLUME ["/app/data", "/app/uploads"]
ENTRYPOINT ["./ninimenu"]

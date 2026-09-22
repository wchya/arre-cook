# syntax=docker/dockerfile:1

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
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/ninimenu ./cmd/server

# ---------- Stage 3: runtime ----------
FROM alpine:3.20
# apk 换腾讯云内网镜像，避免 dl-cdn.alpinelinux.org 境外下载卡住
RUN sed -i 's#https://dl-cdn.alpinelinux.org#https://mirrors.cloud.tencent.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 1000 ninimenu
WORKDIR /app
COPY --from=backend-build /out/ninimenu ./ninimenu
# 前端产物（SPA）
COPY --from=backend-build /src/static ./static
# 种子菜品图片；首次挂载空具名卷时由 Docker 自动拷贝进卷
COPY backend/uploads ./uploads
ENV PORT=8080 GIN_MODE=release TZ=Asia/Shanghai
EXPOSE 8080
USER ninimenu
VOLUME ["/app/data", "/app/uploads"]
ENTRYPOINT ["./ninimenu"]

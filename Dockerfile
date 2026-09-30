# syntax=docker/dockerfile:1

# ---- Go build stage ----
# cross-compile on the build host's platform: no QEMU emulation for the Go build
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/music-tag-server .

# ---- runtime stage ----
FROM python:3.12-slim
LABEL title="Music Tag Web | 音乐标签网页版 (Go)"
LABEL description="『音乐标签』Web版是一款可以编辑歌曲的标题，专辑，艺术家，歌词，封面等信息的应用程序"
LABEL authors="xhongc"

# Python side: audio tag I/O only (mutagen via component/music_tag, requests for cover downloads)
RUN pip install --no-cache-dir mutagen requests

WORKDIR /app
COPY --from=build /out/music-tag-server ./music-tag-server
COPY server/py ./server/py
COPY component ./component
COPY static/dist ./static/dist

ENV PORT=8002 \
    MEDIA_ROOT=/app/media \
    DATA_DIR=/app/data \
    STATIC_DIR=/app/static \
    PYTHON_BIN=python3

EXPOSE 8002
VOLUME ["/app/media", "/app/data"]

CMD ["./music-tag-server"]

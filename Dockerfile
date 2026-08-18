# 构建阶段：使用与生成机一致的 Go 镜像，离线 vendor 构建
FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o /out/gosched ./...

# 运行阶段：alpine 3.20，固定运行时镜像
FROM docker.m.daocloud.io/library/alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/gosched /usr/local/bin/gosched

EXPOSE 8080
ENTRYPOINT ["gosched"]
CMD ["--addr", ":8080"]

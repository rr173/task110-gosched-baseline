# 构建阶段：使用与生成机一致的 Go 镜像，Go module 模式构建
FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm AS build

WORKDIR /src

ENV GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/gosched .

# 运行阶段：alpine 3.20，固定运行时镜像
FROM docker.m.daocloud.io/library/alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/gosched /usr/local/bin/gosched

EXPOSE 8080
ENTRYPOINT ["gosched"]
CMD ["--addr", ":8080"]

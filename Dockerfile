# 使用官方的Go镜像作为构建环境
FROM golang:1.20-buster as builder

# 设置工作目录
WORKDIR /app

# 复制Go mod和sum文件
COPY go.mod go.sum ./

# 下载所有依赖项
RUN go mod download

# 复制源代码到容器内部
COPY . .

# 编译应用
# CGO_ENABLED=0 命令是为了确保产生一个静态二进制文件
# -o 指定输出的二进制文件的名字
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main .

# 使用scratch作为轻量级的基础镜像
FROM scratch

# 从构建环境中复制二进制文件和其他必要的文件
COPY --from=builder /app/main /main
COPY --from=builder /app/mrt.json /mrt.json
COPY --from=builder /app/cache.json /cache.json

# 指定容器启动时运行的命令
CMD ["/main"]
# 设置底层
FROM golang:1.22-alpine

#设置工作目录
WORKDIR /app

#换上国内代理
ENV GOPROXY=https://goproxy.cn,direct

# 容器内安装热重载引擎Air
RUN go install github.com/air-verse/air@latest

CMD [ "air" ]
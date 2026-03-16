.PHONY: gen build clean

# 变量定义，方便后续修改
PROTO_DIR=api

# 1. 生成代码任务
gen:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/dfs.proto

# 2. 编译任务（编译出 Master 和 Volume 两个二进制文件）
build:
	go build -o bin/master cmd/master/main.go
	go build -o bin/volume cmd/volume/main.go

# 3. 清理任务
clean:
	rm -rf bin/
	rm api/*.pb.go

# 4. 默认执行的任务
all: gen build

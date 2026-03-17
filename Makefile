.PHONY: all gen build build-master build-volume build-client test test-unit test-bench clean run-master run-volume run-client help

# 变量定义
PROTO_DIR=api
BIN_DIR=bin
MASTER_ADDR=localhost:50051

# 默认任务
all: gen build

# 生成 protobuf 代码
gen:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/dfs.proto

# 编译所有组件
build: build-master build-volume build-client
	@echo "All components built successfully."

# 编译 Master
build-master:
	@echo "Building Master..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/master cmd/master/main.go

# 编译 Volume
build-volume:
	@echo "Building Volume..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/volume cmd/volume/main.go

# 编译 Client
build-client:
	@echo "Building Client..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/client cmd/client/main.go

# 运行测试
test: test-unit

test-unit:
	@echo "Running unit tests..."
	go test ./internal/... -v

test-bench:
	@echo "Running benchmarks..."
	go test ./internal/bench/... -bench=. -benchmem -run=^$$

# 运行组件（开发模式）
run-master:
	@echo "Starting Master on port 50051..."
	go run cmd/master/main.go -port=50051

run-volume-1:
	@echo "Starting Volume-1 on port 50052..."
	go run cmd/volume/main.go -id=vol-1 -port=50052 -master=$(MASTER_ADDR)

run-volume-2:
	@echo "Starting Volume-2 on port 50053..."
	go run cmd/volume/main.go -id=vol-2 -port=50053 -master=$(MASTER_ADDR)

run-volume-3:
	@echo "Starting Volume-3 on port 50054..."
	go run cmd/volume/main.go -id=vol-3 -port=50054 -master=$(MASTER_ADDR)

# 客户端操作
upload:
	@echo "Uploading file: $(FILE)"
	go run cmd/client/main.go -action=upload -file=$(FILE) -master=$(MASTER_ADDR)

download:
	@echo "Downloading file: $(FILE)"
	go run cmd/client/main.go -action=download -file=$(FILE) -master=$(MASTER_ADDR)

# 清理
clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR)/
	rm -f api/*.pb.go
	rm -f persist.log
	rm -rf data/*

# 代码检查
lint:
	@echo "Running linter..."
	gofmt -l .
	go vet ./...

# 格式化代码
fmt:
	@echo "Formatting code..."
	gofmt -w .

# 帮助信息
help:
	@echo "Available targets:"
	@echo "  make all          - Generate protobuf and build all components"
	@echo "  make gen          - Generate protobuf code"
	@echo "  make build        - Build all components"
	@echo "  make build-master - Build Master node only"
	@echo "  make build-volume - Build Volume node only"
	@echo "  make build-client - Build Client only"
	@echo "  make test         - Run all tests"
	@echo "  make test-unit    - Run unit tests"
	@echo "  make test-bench   - Run benchmarks"
	@echo "  make run-master   - Run Master node (dev mode)"
	@echo "  make run-volume-1 - Run Volume-1 (dev mode)"
	@echo "  make upload FILE=test.txt  - Upload a file"
	@echo "  make download FILE=test.txt - Download a file"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make fmt          - Format Go code"
	@echo "  make help         - Show this help message"

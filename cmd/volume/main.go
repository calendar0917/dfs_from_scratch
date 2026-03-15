package main

import (
	"log"
	"net"

	"go-dfs/api"
	"go-dfs/internal/service"

	"google.golang.org/grpc"
)

func main() {
	// 1. 监听端口
	lis, err := net.Listen("tcp", ":50052") // 假设 Volume 监听 50052
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 2. 创建 gRPC 服务器实例
	s := grpc.NewServer()

	// 3. 实例化逻辑层（管理员）
	volumeLogic := &service.VolumeServer{
		StorageDir: "./data/volume_v1", // 确保这个目录存在
	}

	// 4. 将逻辑层注册到 gRPC 服务器中
	// 把“自动化流水线”和“仓库管理员”连接起来
	api.RegisterVolumeServiceServer(s, volumeLogic)

	log.Printf("Volume Server 正在监听: %v", lis.Addr())

	// 5. 启动服务
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

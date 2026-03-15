package main

import (
	"flag"
	"log"
	"net"

	"go-dfs/api"
	"go-dfs/internal/service"

	"google.golang.org/grpc"
)

func main() {
	// 定义命令行参数，默认监听 50051
	port := flag.String("port", "50051", "Master 监听端口")
	flag.Parse()

	// 开启网络监听
	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("【Master】网络监听失败: %v", err)
	}

	// 创建 gRPC 服务实例
	s := grpc.NewServer()

	// 初始化 Master 逻辑层
	// 注意：这里一定要用之前写的 NewMasterServer()，确保内部的 map 被初始化了
	masterLogic := service.NewMasterServer()

	// 将逻辑注册到 gRPC 服务中
	api.RegisterMasterServiceServer(s, masterLogic)

	log.Printf("Master Server 已启动，正在监听端口: %s", *port)

	// 6. 启动服务（阻塞运行）
	if err := s.Serve(lis); err != nil {
		log.Fatalf("【Master】服务崩溃: %v", err)
	}
}

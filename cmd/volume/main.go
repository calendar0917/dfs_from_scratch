package main

import (
	"context"
	"flag"
	"log"
	"net"
	"time"

	"go-dfs/api"
	"go-dfs/internal/service"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func registerWithMaster(nodeID, myAddr, masterAddr string) {
	// 和 master 建立连接
	conn, err := grpc.Dial(masterAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("无法连接 Master %v", err)
		return
	}
	defer conn.Close()
	// 注册，带重试
	client := api.NewMasterServiceClient(conn)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		_, err := client.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  nodeID,
			Address: myAddr,
		})
		cancel()
		if err == nil {
			log.Printf("成功注册到 Master")
			break
		}
		log.Printf("注册失败： %v，2 秒后重试")
		time.Sleep(time.Second * 2)
	}
}

func main() {
	// 用 flag 包来进行参数读取，而不是硬编码
	nodeID := flag.String("id", "vol-1", "节点唯一标识")
	port := flag.String("port", "50052", "Volumn 监听端口")
	masterAddr := flag.String("master", "localhost:50051", "Master 地址")
	// 自己先监听端口，然后才能向 Master 注册
	lis, _ := net.Listen("tcp", ":"+*port)
	s := grpc.NewServer()
	api.RegisterVolumeServiceServer(s, &service.VolumeServer{StorageDir: "./data/" + *nodeID})
	// 用一个协程来进行打印
	go func() {
		log.Println("Volume Server %s 启动，监听端口: %s", *nodeID, *port)
		if err := s.Serve(lis); err != nil {
			log.Fatalf("服务崩溃")
		}
	}()
	// 注册服务到 Master
	registerWithMaster(*nodeID, "localhost:"+*port, *masterAddr)
	// 阻塞主线程，防止服务退出
	select {}
}

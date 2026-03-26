package main

import (
	"context"
	"flag"
	"log"
	"net"
	"time"

	"go-dfs/api"
	runtimecfg "go-dfs/internal/runtime"
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
	client := api.NewMasterServiceClient(conn)
	// 使用 Ticker 每 3 秒跳动一次
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// 立即执行一次注册（不用等 3 秒）
	doRegister := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := client.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  nodeID,
			Address: myAddr,
		})
		if err != nil {
			log.Printf("注册/心跳失败: %v", err)
			return false
		}
		return true
	}

	// 先做第一次注册
	for !doRegister() {
		log.Println("等待 2 秒后重试初始注册...")
		time.Sleep(2 * time.Second)
	}
	log.Println("初始注册成功，进入心跳模式")

	// 周期性发送心跳
	for range ticker.C {
		doRegister()
	}
}

func newVolumeServerForMain(nodeID, storageDir string) *service.VolumeServer {
	return service.NewVolumeServer(runtimecfg.ResolveVolumeStorageDir(nodeID, storageDir))
}

func main() {
	// 用 flag 包来进行参数读取，而不是硬编码
	nodeID := flag.String("id", "vol-1", "节点唯一标识")
	port := flag.String("port", "50052", "Volumn 监听端口")
	masterAddr := flag.String("master", "localhost:50051", "Master 地址")
	storageDir := flag.String("storage-dir", "", "Volume 本地存储目录")
	flag.Parse()
	// 自己先监听端口，然后才能向 Master 注册
	lis, err := runtimecfg.MustListen(net.Listen, "tcp", ":"+*port)
	if err != nil {
		log.Fatalf("【Volume】网络监听失败: %v", err)
	}
	s := grpc.NewServer()
	api.RegisterVolumeServiceServer(s, newVolumeServerForMain(*nodeID, *storageDir))
	// 用一个协程来进行打印
	go func() {
		log.Printf("Volume Server %s 启动，监听端口: %s", *nodeID, *port)
		if err := s.Serve(lis); err != nil {
			log.Fatalf("服务崩溃")
		}
	}()
	// 注册服务到 Master
	registerWithMaster(*nodeID, "localhost:"+*port, *masterAddr)
	// 阻塞主线程，防止服务退出
	select {}
}

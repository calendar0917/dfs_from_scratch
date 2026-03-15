package main

import (
	"context"
	"log"
	"time"

	"go-dfs/api" // 替换成你的模块名

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1. 建立连接
	// insecure.NewCredentials() 表示不使用 SSL/TLS 加密（开发环境常用）
	conn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("无法连接服务器: %v", err)
	}
	defer conn.Close()

	// 2. 创建客户端“存根” (Stub)
	// NewVolumeServiceClient 是 protoc 自动生成的
	client := api.NewVolumeServiceClient(conn)

	// 3. 准备要上传的数据
	req := &api.UploadRequest{
		Filename: "hello_distributed.txt",
		Content:  []byte("你好，这是我的第一个分布式文件系统测试数据！"),
	}

	// 4. 调用远程方法
	// 设置 5 秒超时，工程实践中习惯
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	resp, err := client.UploadFile(ctx, req)
	if err != nil {
		log.Fatalf("上传失败: %v", err)
	}

	// 5. 打印结果
	if resp.Success {
		log.Printf("上传成功！文件 ID: %s", resp.FileId)
	} else {
		log.Printf("上传失败。")
	}
}

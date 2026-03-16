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
	// 问路：联系 Master
	// 建议：地址也可以通过 flag 传入
	masterConn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("无法连接 Master: %v", err)
	}
	defer masterConn.Close()

	mClient := api.NewMasterServiceClient(masterConn)

	// 设置超时并立即执行
	mCtx, mCancel := context.WithTimeout(context.Background(), time.Second*5)
	respMaster, err := mClient.AssignVolume(mCtx, &api.AssignVolumeRequest{
		Filename: "hello_dist.txt",
		FileSize: 100,
	})
	mCancel() // 重点：用完即毁

	if err != nil {
		log.Fatalf("Master 分配节点失败: %v", err)
	}

	log.Printf("【Client】Master 指派节点: %s", respMaster.Address)

	// 联系对应的 Volume
	// 现在的 volume 是名单列表
	vConn, err := grpc.NewClient(respMaster.Address[0], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("无法连接 Volume 节点 %s: %v", respMaster.Address, err)
	}
	defer vConn.Close()

	vClient := api.NewVolumeServiceClient(vConn)
	vCtx, vCancel := context.WithTimeout(context.Background(), time.Second*10) // 上传文件可以给久一点

	respVolume, err := vClient.UploadFile(vCtx, &api.UploadRequest{
		Filename:    "hello_lianshi.txt",
		Content:     []byte("test"),
		NextTargets: respMaster.Address,
	})
	vCancel()

	if err != nil || !respVolume.Success {
		log.Fatalf("上传文件失败: %v", err)
	}

	log.Printf("上传成功！文件 ID: %s", respVolume.FileId)
}
